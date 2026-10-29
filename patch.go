package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// A patch source is a web folder holding manifest.json and the files it lists:
//
//	{ "version": "2026.10.28",
//	  "files": [ { "path": "Data/GameData/Custom-WorldData.wad", "size": 1234, "sha256": "ab12..." } ] }
//
// Each file is downloaded from <folder>/<path> into the same path under the game folder.
// Only game data may arrive this way; programs and scripts never do.

type manifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	Version string         `json:"version"`
	Files   []manifestFile `json:"files"`
}

var allowedExt = map[string]bool{
	".wad": true, ".xml": true, ".bin": true, ".dat": true, ".lang": true, ".txt": true, ".json": true, ".ini": true,
	".mp3": true, ".ogg": true, ".wav": true, ".dds": true, ".nif": true, ".kf": true, ".kfm": true, ".png": true,
	".jpg": true, ".jpeg": true, ".gui": true, ".ttf": true, ".bik": true, ".csv": true,
}

const (
	maxFiles    = 5000
	maxFileSize = 4 << 30 // 4 GB
)

// cleanPatchPath checks a path from a manifest and returns it in forward-slash form, or an error.
func cleanPatchPath(p string) (string, error) {
	if p == "" || strings.ContainsAny(p, "\\:\x00") || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("unsafe path %q", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return "", fmt.Errorf("unsafe path %q", p)
		}
	}
	if !strings.HasPrefix(p, "Data/") && !strings.HasPrefix(p, "Bin/") {
		return "", fmt.Errorf("%q is outside Data and Bin", p)
	}
	if !allowedExt[strings.ToLower(path.Ext(p))] {
		return "", fmt.Errorf("%q is not a game data file", p)
	}
	return p, nil
}

func manifestURL(base string) (string, string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", "", err
	}
	if strings.HasSuffix(strings.ToLower(u.Path), ".json") {
		dir := *u
		dir.Path = path.Dir(u.Path) + "/"
		return u.String(), dir.String(), nil
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	m := *u
	m.Path += "manifest.json"
	return m.String(), u.String(), nil
}

func fetchManifest(base string) (*manifest, string, error) {
	murl, dir, err := manifestURL(base)
	if err != nil {
		return nil, "", errors.New("The patch files address isn't valid.")
	}
	req, _ := http.NewRequest("GET", murl, nil)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "SpiralNavigator/"+version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("Couldn't reach %s for patches.", req.URL.Host)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, "", errors.New("This server has no patch list (manifest.json wasn't found).")
	}
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("The patch server answered with an error (%d).", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, "", errors.New("The patch list download stopped partway.")
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, "", errors.New("The patch list isn't valid JSON.")
	}
	if len(m.Files) > maxFiles {
		return nil, "", errors.New("The patch list has too many files.")
	}
	for i := range m.Files {
		f := &m.Files[i]
		clean, err := cleanPatchPath(f.Path)
		if err != nil {
			return nil, "", fmt.Errorf("The patch list contains a file that can't be installed: %v.", err)
		}
		f.Path = clean
		f.SHA256 = strings.ToLower(f.SHA256)
		if len(f.SHA256) != 64 {
			return nil, "", fmt.Errorf("The patch list has no checksum for %s.", f.Path)
		}
		if f.Size < 0 || f.Size > maxFileSize {
			return nil, "", fmt.Errorf("%s has an unreasonable size.", f.Path)
		}
	}
	return &m, dir, nil
}

// ---- Hashing, with a memory of what was hashed so big wads aren't read again and again ----

type hashEntry struct {
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Sum   string `json:"sum"`
}

type hashCache struct {
	mu   sync.Mutex
	path string
	m    map[string]hashEntry
}

func newHashCache(dir string) *hashCache {
	h := &hashCache{path: filepath.Join(dir, "hashes.json"), m: map[string]hashEntry{}}
	if b, err := os.ReadFile(h.path); err == nil {
		_ = json.Unmarshal(b, &h.m)
	}
	return h
}

func (h *hashCache) save() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if b, err := json.Marshal(h.m); err == nil {
		_ = os.WriteFile(h.path, b, 0o644)
	}
}

func (h *hashCache) sum(full string, st os.FileInfo) (string, error) {
	h.mu.Lock()
	e, ok := h.m[full]
	h.mu.Unlock()
	if ok && e.Size == st.Size() && e.MTime == st.ModTime().UnixNano() {
		return e.Sum, nil
	}
	f, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hs := sha256.New()
	if _, err := io.Copy(hs, f); err != nil {
		return "", err
	}
	s := hex.EncodeToString(hs.Sum(nil))
	h.mu.Lock()
	h.m[full] = hashEntry{st.Size(), st.ModTime().UnixNano(), s}
	h.mu.Unlock()
	return s, nil
}

// ---- Checking what's out of date ----

type PatchFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type PatchCheck struct {
	Version string      `json:"version"`
	Files   []PatchFile `json:"files"`
	Bytes   int64       `json:"bytes"`
	Total   int         `json:"total"`
}

type plan struct {
	dir   string
	files []manifestFile
}

func safeJoin(root, rel string) (string, error) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	r, err := filepath.Rel(root, full)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", fmt.Errorf("%s would land outside the game folder", rel)
	}
	return full, nil
}

func (s *store) checkPatches(p Profile, folder string) (*PatchCheck, *plan, error) {
	if p.PatchURL == "" {
		return nil, nil, errors.New("This server has no patch files address. Add one in the server's settings.")
	}
	if !isGameFolder(folder) {
		return nil, nil, errors.New("Wizard101 wasn't found. Choose its folder in Settings first.")
	}
	m, dir, err := fetchManifest(p.PatchURL)
	if err != nil {
		return nil, nil, err
	}
	hc := newHashCache(s.dir)
	defer hc.save()

	check := &PatchCheck{Version: m.Version, Total: len(m.Files), Files: []PatchFile{}}
	pl := &plan{dir: dir}
	for _, f := range m.Files {
		full, err := safeJoin(folder, f.Path)
		if err != nil {
			return nil, nil, err
		}
		st, err := os.Stat(full)
		if err == nil && !st.IsDir() && st.Size() == f.Size {
			if sum, e := hc.sum(full, st); e == nil && sum == f.SHA256 {
				continue
			}
		}
		check.Files = append(check.Files, PatchFile{f.Path, f.Size})
		check.Bytes += f.Size
		pl.files = append(pl.files, f)
	}
	sort.Slice(check.Files, func(i, j int) bool { return check.Files[i].Path < check.Files[j].Path })
	return check, pl, nil
}

// ---- Downloading ----

type PatchStatus struct {
	State     string `json:"state"` // idle, running, done, error
	File      string `json:"file"`
	FileIndex int    `json:"fileIndex"`
	FileCount int    `json:"fileCount"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"`
	Error     string `json:"error,omitempty"`
}

type patcher struct {
	mu sync.Mutex
	st PatchStatus
}

func (pt *patcher) status() PatchStatus {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.st
}

func (pt *patcher) set(f func(*PatchStatus)) {
	pt.mu.Lock()
	f(&pt.st)
	pt.mu.Unlock()
}

// start begins a download in the background. It returns false if one is already running.
func (pt *patcher) start(folder string, pl *plan, total int64) bool {
	pt.mu.Lock()
	if pt.st.State == "running" {
		pt.mu.Unlock()
		return false
	}
	pt.st = PatchStatus{State: "running", FileCount: len(pl.files), Total: total}
	pt.mu.Unlock()

	go func() {
		client := &http.Client{CheckRedirect: httpClient.CheckRedirect} // no overall timeout: wads are big
		for i, f := range pl.files {
			pt.set(func(s *PatchStatus) { s.File = f.Path; s.FileIndex = i + 1 })
			if err := pt.download(client, folder, pl.dir, f); err != nil {
				msg := err.Error()
				pt.set(func(s *PatchStatus) { s.State = "error"; s.Error = msg })
				return
			}
		}
		pt.set(func(s *PatchStatus) { s.State = "done"; s.Done = s.Total })
	}()
	return true
}

type countWriter struct {
	pt *patcher
	w  io.Writer
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.pt.set(func(s *PatchStatus) { s.Done += int64(n) })
	return n, err
}

func (pt *patcher) download(client *http.Client, folder, baseDir string, f manifestFile) error {
	full, err := safeJoin(folder, f.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("Couldn't create the folder for %s: %v", f.Path, err)
	}
	segs := strings.Split(f.Path, "/")
	for i := range segs {
		segs[i] = url.PathEscape(segs[i])
	}
	req, _ := http.NewRequest("GET", baseDir+strings.Join(segs, "/"), nil)
	req.Header.Set("User-Agent", "SpiralNavigator/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Couldn't download %s: the patch server didn't answer.", f.Path)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Couldn't download %s: the patch server answered %d.", f.Path, resp.StatusCode)
	}
	part := full + ".part"
	out, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("Couldn't write %s: %v", f.Path, err)
	}
	hs := sha256.New()
	n, err := io.Copy(io.MultiWriter(&countWriter{pt, out}, hs), io.LimitReader(resp.Body, f.Size+1))
	cerr := out.Close()
	if err != nil || cerr != nil {
		_ = os.Remove(part)
		return fmt.Errorf("The download of %s stopped partway.", f.Path)
	}
	if n != f.Size || hex.EncodeToString(hs.Sum(nil)) != f.SHA256 {
		_ = os.Remove(part)
		return fmt.Errorf("%s didn't match its checksum, so it was not installed.", f.Path)
	}
	if err := os.Rename(part, full); err != nil {
		_ = os.Remove(part)
		return fmt.Errorf("Couldn't replace %s (is the game running?): %v", f.Path, err)
	}
	return nil
}

// ---- Making a patch list from a folder: launcher make-manifest <folder> ----

func makeManifest(dir, ver string) error {
	var files []manifestFile
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if _, e := cleanPatchPath(rel); e != nil {
			if rel != "manifest.json" {
				fmt.Printf("skipped %s (%v)\n", rel, e)
			}
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		hs := sha256.New()
		if _, err := io.Copy(hs, f); err != nil {
			return err
		}
		files = append(files, manifestFile{rel, info.Size(), hex.EncodeToString(hs.Sum(nil))})
		fmt.Printf("added %s\n", rel)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if ver == "" {
		ver = time.Now().Format("2006.01.02")
	}
	b, _ := json.MarshalIndent(manifest{Version: ver, Files: files}, "", "  ")
	out := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("\nWrote %s with %d files. Upload this folder to your web host.\n", out, len(files))
	return nil
}
