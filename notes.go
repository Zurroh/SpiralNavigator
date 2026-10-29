package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxNotesBytes = 1 << 20 // 1 MB is far more than a page of notes needs

// Notes is what the page shows for a server's patch notes.
type Notes struct {
	Markdown  string `json:"markdown"`
	FetchedAt int64  `json:"fetchedAt"` // unix seconds of the copy shown
	FromCache bool   `json:"fromCache"`
	Error     string `json:"error,omitempty"`
}

var httpClient = &http.Client{
	Timeout: 12 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

func (s *store) notesPaths(id string) (string, string) {
	d := filepath.Join(s.dir, "notes")
	_ = os.MkdirAll(d, 0o755)
	return filepath.Join(d, id+".md"), filepath.Join(d, id+".json")
}

type notesMeta struct {
	FetchedAt int64  `json:"fetchedAt"`
	URL       string `json:"url"`
}

// getNotes fetches the profile's notes file and keeps the copy, so the last good notes show when the host is down.
func (s *store) getNotes(p Profile) Notes {
	if p.NotesURL == "" {
		return Notes{Error: "none"}
	}
	mdPath, metaPath := s.notesPaths(p.ID)

	text, err := fetchText(p.NotesURL)
	if err == nil {
		now := time.Now().Unix()
		_ = os.WriteFile(mdPath, []byte(text), 0o644)
		mb, _ := json.Marshal(notesMeta{FetchedAt: now, URL: p.NotesURL})
		_ = os.WriteFile(metaPath, mb, 0o644)
		return Notes{Markdown: text, FetchedAt: now}
	}

	// Offline or the host is down: show the last copy, and say so.
	if b, e := os.ReadFile(mdPath); e == nil {
		var meta notesMeta
		if mb, e := os.ReadFile(metaPath); e == nil {
			_ = json.Unmarshal(mb, &meta)
		}
		if meta.URL == "" || meta.URL == p.NotesURL {
			return Notes{Markdown: string(b), FetchedAt: meta.FetchedAt, FromCache: true, Error: err.Error()}
		}
	}
	return Notes{Error: err.Error()}
}

// fetchText downloads a small text file. A "host/path" address is tried over https first, then http.
func fetchText(raw string) (string, error) {
	text, err := fetchOnce(raw)
	if err == nil {
		return text, nil
	}
	// An https address on a host with no certificate (a plain home server) is retried over http.
	if u, e := url.Parse(raw); e == nil && u.Scheme == "https" && isConnectError(err) {
		u.Scheme = "http"
		if t, e := fetchOnce(u.String()); e == nil {
			return t, nil
		}
	}
	return "", err
}

// transportError marks a failure to talk to the host at all (as opposed to the host answering with an error).
type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }

func isConnectError(err error) bool {
	var te *transportError
	return errors.As(err, &te)
}

func fetchOnce(raw string) (string, error) {
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return "", errors.New("The patch notes address isn't valid.")
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "SpiralNavigator/"+version)
	req.Header.Set("Accept", "text/markdown, text/plain, */*;q=0.5")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", &transportError{fmt.Sprintf("Couldn't reach %s.", req.URL.Host)}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return "", errors.New("The notes file wasn't found (404). Check the address.")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("The notes address answered with an error (%d).", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxNotesBytes+1))
	if err != nil {
		return "", errors.New("The download of the notes stopped partway.")
	}
	if len(body) > maxNotesBytes {
		return "", errors.New("The notes file is larger than 1 MB.")
	}
	text := strings.TrimPrefix(string(body), "\ufeff")
	head := strings.ToLower(strings.TrimSpace(firstN(text, 200)))
	if strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html") {
		return "", errors.New("That address gave back a web page, not a text file.")
	}
	return text, nil
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
