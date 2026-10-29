package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed ui
var uiFS embed.FS

type app struct {
	st       *store
	pt       *patcher
	token    string
	addr     string // 127.0.0.1:port
	lastBeat atomic.Int64
	beaten   atomic.Bool
	quit     chan struct{}
	quitOnce sync.Once
	pendMu   sync.Mutex
	pending  *plan // the last check, kept for "install these"
	pendFor  string
}

func newApp(st *store) *app {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &app{st: st, pt: &patcher{st: PatchStatus{State: "idle"}}, token: hex.EncodeToString(b), quit: make(chan struct{})}
}

func (a *app) requestQuit() { a.quitOnce.Do(func() { close(a.quit) }) }

// serve starts the page server on a free port on this computer only and returns the address to open.
func (a *app) serve() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	a.addr = ln.Addr().String()
	sub, _ := fs.Sub(uiFS, "ui")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/state", a.api(a.handleState))
	mux.HandleFunc("/api/save", a.api(a.handleSave))
	mux.HandleFunc("/api/notes", a.api(a.handleNotes))
	mux.HandleFunc("/api/ping", a.api(a.handlePing))
	mux.HandleFunc("/api/play", a.api(a.handlePlay))
	mux.HandleFunc("/api/patch/check", a.api(a.handlePatchCheck))
	mux.HandleFunc("/api/patch/start", a.api(a.handlePatchStart))
	mux.HandleFunc("/api/patch/status", a.api(a.handlePatchStatus))
	mux.HandleFunc("/api/game", a.api(a.handleGame))
	mux.HandleFunc("/api/browse", a.api(a.handleBrowse))
	mux.HandleFunc("/api/open", a.api(a.handleOpen))
	mux.HandleFunc("/api/share", a.api(a.handleShare))
	mux.HandleFunc("/api/import", a.api(a.handleImport))
	mux.HandleFunc("/api/beat", a.api(a.handleBeat))
	mux.HandleFunc("/api/quit", a.api(func(w http.ResponseWriter, r *http.Request) {
		a.requestQuit()
		writeJSON(w, map[string]bool{"ok": true})
	}))

	go http.Serve(ln, a.guard(mux))
	return "http://" + a.addr + "/?k=" + a.token, nil
}

// guard keeps other programs and web pages out: only this computer, only this launcher's own page.
func (a *app) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != a.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// The first visit carries the key; keep it in a cookie and clean the address.
		if k := r.URL.Query().Get("k"); k != "" && r.URL.Path == "/" {
			if subtle.ConstantTimeCompare([]byte(k), []byte(a.token)) == 1 {
				http.SetCookie(w, &http.Cookie{Name: "k", Value: a.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		c, err := r.Cookie("k")
		if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(a.token)) != 1 {
			http.Error(w, "Open the launcher from its own window.", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https: http:; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// api wraps a handler: POST/GET only, JSON in and out, and refuses requests that don't come from the page itself.
func (a *app) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+a.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// ---- handlers ----

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	cfg := a.st.snapshot()
	sel, _ := a.st.profile(cfg.Selected)
	folder := gameFolderFor(cfg, sel)
	writeJSON(w, map[string]any{
		"config":     cfg,
		"gameFolder": folder,
		"version":    version,
	})
}

func (a *app) handleSave(w http.ResponseWriter, r *http.Request) {
	var c Config
	if err := readJSON(r, &c); err != nil {
		writeErr(w, "That didn't save.")
		return
	}
	if err := a.st.replace(c); err != nil {
		writeErr(w, err.Error())
		return
	}
	a.handleState(w, r)
}

func (a *app) handleNotes(w http.ResponseWriter, r *http.Request) {
	p, ok := a.st.profile(r.URL.Query().Get("id"))
	if !ok {
		writeErr(w, "That server isn't in your list.")
		return
	}
	writeJSON(w, a.st.getNotes(p))
}

func (a *app) handlePing(w http.ResponseWriter, r *http.Request) {
	p, ok := a.st.profile(r.URL.Query().Get("id"))
	if !ok {
		writeErr(w, "That server isn't in your list.")
		return
	}
	up, ms := ping(p.Address, p.Port)
	writeJSON(w, map[string]any{"online": up, "ms": ms})
}

func (a *app) handlePlay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, "That didn't work.")
		return
	}
	p, ok := a.st.profile(req.ID)
	if !ok {
		writeErr(w, "That server isn't in your list.")
		return
	}
	cfg := a.st.snapshot()
	folder := gameFolderFor(cfg, p)
	if isLiveRealm(p.Address) {
		writeErr(w, "That's the live Wizard101 game. This launcher only plays on private servers.")
		return
	}
	if !req.Force {
		if up, _ := ping(p.Address, p.Port); !up {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   fmt.Sprintf("Can't reach %s on port %d. If this server is on Tailscale, make sure Tailscale is running and signed in. Otherwise the server may be off.", p.Address, p.Port),
				"offline": true,
			})
			return
		}
	}
	if err := launchGame(folder, p); err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "closeOnLaunch": cfg.CloseOnLaunch})
}

func (a *app) handlePatchCheck(w http.ResponseWriter, r *http.Request) {
	p, ok := a.st.profile(r.URL.Query().Get("id"))
	if !ok {
		writeErr(w, "That server isn't in your list.")
		return
	}
	folder := gameFolderFor(a.st.snapshot(), p)
	check, pl, err := a.st.checkPatches(p, folder)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	a.pendMu.Lock()
	a.pending, a.pendFor = pl, p.ID
	a.pendMu.Unlock()
	writeJSON(w, check)
}

func (a *app) handlePatchStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, "That didn't work.")
		return
	}
	p, ok := a.st.profile(req.ID)
	if !ok {
		writeErr(w, "That server isn't in your list.")
		return
	}
	a.pendMu.Lock()
	pl, id := a.pending, a.pendFor
	a.pendMu.Unlock()
	if pl == nil || id != p.ID {
		writeErr(w, "Check for patches first.")
		return
	}
	var total int64
	for _, f := range pl.files {
		total += f.Size
	}
	folder := gameFolderFor(a.st.snapshot(), p)
	if !a.pt.start(folder, pl, total) {
		writeErr(w, "A patch is already installing.")
		return
	}
	a.pendMu.Lock()
	a.pending = nil
	a.pendMu.Unlock()
	writeJSON(w, a.pt.status())
}

func (a *app) handlePatchStatus(w http.ResponseWriter, r *http.Request) { writeJSON(w, a.pt.status()) }

func (a *app) handleGame(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Folder string `json:"folder"`
	}
	_ = readJSON(r, &req)
	folder := strings.TrimSpace(req.Folder)
	if folder == "" {
		folder = detectGameFolder()
		if folder == "" {
			writeJSON(w, map[string]any{"found": false})
			return
		}
	}
	writeJSON(w, map[string]any{"found": isGameFolder(folder), "folder": folder})
}

func (a *app) handleBrowse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Start string `json:"start"`
	}
	_ = readJSON(r, &req)
	writeJSON(w, map[string]string{"folder": browseFolder(req.Start)})
}

func (a *app) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &req); err != nil || openURL(req.URL) != nil {
		writeErr(w, "That link can't be opened.")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *app) handleShare(w http.ResponseWriter, r *http.Request) {
	p, ok := a.st.profile(r.URL.Query().Get("id"))
	if !ok {
		writeErr(w, "That server isn't in your list.")
		return
	}
	writeJSON(w, map[string]string{"code": shareCode(p)})
}

func (a *app) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, "That didn't work.")
		return
	}
	p, err := parseShared(req.Text)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, p)
}

func (a *app) handleBeat(w http.ResponseWriter, r *http.Request) {
	a.lastBeat.Store(time.Now().Unix())
	a.beaten.Store(true)
	writeJSON(w, map[string]bool{"ok": true})
}
