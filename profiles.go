package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Profile is one server the launcher can play on.
type Profile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
	NotesURL   string `json:"notesUrl"`
	PatchURL   string `json:"patchUrl"`
	BugURL     string `json:"bugUrl"`
	AutoPatch  bool   `json:"autoPatch"`
	School     string `json:"school"`
	GameFolder string `json:"gameFolder"`
}

// Config is everything the launcher remembers.
type Config struct {
	Version       int       `json:"version"`
	Profiles      []Profile `json:"profiles"`
	Selected      string    `json:"selected"`
	GameFolder    string    `json:"gameFolder"`
	CloseOnLaunch bool      `json:"closeOnLaunch"`
}

var schools = map[string]bool{
	"fire": true, "ice": true, "storm": true, "myth": true, "life": true, "death": true, "balance": true,
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.\-]*[A-Za-z0-9])?$`)

type store struct {
	mu   sync.Mutex
	dir  string
	path string
	cfg  Config
}

func configDir() string {
	if d := os.Getenv("SPIRAL_NAVIGATOR_HOME"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return filepath.Join(base, "SpiralNavigator")
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// loadStore reads the saved config. On the very first run it is seeded from profiles.json beside the program,
// else from the profile built into the program.
func loadStore(seed []byte) (*store, error) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &store{dir: dir, path: filepath.Join(dir, "launcher.json")}
	data, err := os.ReadFile(s.path)
	if err == nil {
		if json.Unmarshal(data, &s.cfg) == nil {
			s.normalizeLocked()
			return s, nil
		}
		_ = os.Rename(s.path, s.path+".broken")
	}

	// First run.
	if exe, e := os.Executable(); e == nil {
		if b, e := os.ReadFile(filepath.Join(filepath.Dir(exe), "profiles.json")); e == nil {
			seed = b
		}
	}
	_ = json.Unmarshal(seed, &s.cfg)
	s.normalizeLocked()
	if len(s.cfg.Profiles) > 0 {
		s.cfg.Selected = s.cfg.Profiles[0].ID
	}
	return s, s.saveLocked()
}

func (s *store) normalizeLocked() {
	s.cfg.Version = 1
	seen := map[string]bool{}
	out := s.cfg.Profiles[:0]
	for _, p := range s.cfg.Profiles {
		if p.ID == "" || seen[p.ID] {
			p.ID = newID()
		}
		seen[p.ID] = true
		if !schools[p.School] {
			p.School = "storm"
		}
		out = append(out, p)
	}
	if out == nil {
		out = []Profile{}
	}
	s.cfg.Profiles = out
	if s.cfg.Selected != "" && !seen[s.cfg.Selected] {
		s.cfg.Selected = ""
	}
	if s.cfg.Selected == "" && len(out) > 0 {
		s.cfg.Selected = out[0].ID
	}
}

func (s *store) saveLocked() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *store) snapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Profiles = append([]Profile{}, s.cfg.Profiles...)
	return c
}

func (s *store) profile(id string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.cfg.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// replace swaps in a whole new config from the page, after checking every profile.
func (s *store) replace(c Config) error {
	if len(c.Profiles) > 200 {
		return errors.New("too many servers")
	}
	for i := range c.Profiles {
		if err := cleanProfile(&c.Profiles[i]); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = c
	s.normalizeLocked()
	return s.saveLocked()
}

// cleanProfile trims and checks one profile; it returns a message fit to show the person.
func cleanProfile(p *Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Address = strings.TrimSpace(p.Address)
	p.NotesURL = strings.TrimSpace(p.NotesURL)
	p.PatchURL = strings.TrimSpace(p.PatchURL)
	p.BugURL = strings.TrimSpace(p.BugURL)
	p.GameFolder = strings.TrimSpace(p.GameFolder)
	if p.Name == "" {
		return errors.New("Give the server a name.")
	}
	if len([]rune(p.Name)) > 60 {
		return errors.New("Keep the server name under 60 characters.")
	}
	host := strings.TrimSuffix(p.Address, ".")
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip == nil && !hostRe.MatchString(host) {
		return errors.New("The address should be an IP address or a host name, like 100.64.0.5 or play.example.com.")
	}
	p.Address = host
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("The port should be a number from 1 to 65535.")
	}
	if p.NotesURL != "" {
		u, err := normalizeWebURL(p.NotesURL)
		if err != nil {
			return errors.New("The patch notes address should start with http:// or https://.")
		}
		p.NotesURL = u
	}
	if p.PatchURL != "" {
		u, err := normalizeWebURL(p.PatchURL)
		if err != nil {
			return errors.New("The patch files address should start with http:// or https://.")
		}
		p.PatchURL = u
	}
	if p.BugURL != "" {
		u, err := normalizeWebURL(p.BugURL)
		if err != nil {
			return errors.New("The bug report address should start with http:// or https://.")
		}
		p.BugURL = u
	}
	if !schools[p.School] {
		p.School = "storm"
	}
	return nil
}

// normalizeWebURL accepts "host/path" (taken as https) or a full http(s) address.
func normalizeWebURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("bad address")
	}
	return u.String(), nil
}

// ---- Sharing a server with a friend: a short text that holds the profile's fields ----

const shareTag = "w101srv:"

type shared struct {
	Name     string `json:"n"`
	Address  string `json:"a"`
	Port     int    `json:"p"`
	NotesURL string `json:"u,omitempty"`
	PatchURL string `json:"f,omitempty"`
	School   string `json:"s,omitempty"`
	BugURL   string `json:"b,omitempty"`
}

func shareCode(p Profile) string {
	b, _ := json.Marshal(shared{p.Name, p.Address, p.Port, p.NotesURL, p.PatchURL, p.School, p.BugURL})
	return shareTag + base64.RawURLEncoding.EncodeToString(b)
}

// parseShared reads a share code, or the plain JSON of a profile, into a new profile.
func parseShared(text string) (Profile, error) {
	text = strings.TrimSpace(text)
	var p Profile
	if strings.HasPrefix(text, shareTag) {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(text, shareTag))
		if err != nil {
			return p, errors.New("That doesn't look like a shared server.")
		}
		var s shared
		if err := json.Unmarshal(raw, &s); err != nil {
			return p, errors.New("That doesn't look like a shared server.")
		}
		p = Profile{Name: s.Name, Address: s.Address, Port: s.Port, NotesURL: s.NotesURL, PatchURL: s.PatchURL, School: s.School, BugURL: s.BugURL}
	} else if err := json.Unmarshal([]byte(text), &p); err != nil {
		return p, errors.New("That doesn't look like a shared server. Paste the text that starts with w101srv:.")
	}
	p.ID = newID()
	p.GameFolder = ""
	p.AutoPatch = false
	if err := cleanProfile(&p); err != nil {
		return p, err
	}
	return p, nil
}
