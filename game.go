package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const clientRel = "Bin/WizardGraphicalClient.exe"

// liveDomains are the real game's servers. This launcher is for private servers, and refuses these.
var liveDomains = []string{"wizard101.com", "kingsisle.com", "gameforge.com"}
var liveHosts = []string{"login.us.wizard101.com", "patch.us.wizard101.com", "login.eu.wizard101.com", "patch.eu.wizard101.com"}

func clientPath(folder string) string {
	return filepath.Join(folder, filepath.FromSlash(clientRel))
}

func isGameFolder(folder string) bool {
	if folder == "" {
		return false
	}
	st, err := os.Stat(clientPath(folder))
	return err == nil && !st.IsDir()
}

// gameFolderFor picks the folder for a profile: its own, then the shared one, then whatever it can find.
func gameFolderFor(cfg Config, p Profile) string {
	for _, f := range []string{p.GameFolder, cfg.GameFolder} {
		if isGameFolder(f) {
			return f
		}
	}
	return detectGameFolder()
}

func detectGameFolder() string {
	var cands []string
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, d, filepath.Dir(d))
	}
	cands = append(cands, registryInstallPaths()...)
	for _, env := range []string{"ProgramData", "ProgramFiles(x86)", "ProgramFiles"} {
		if v := os.Getenv(env); v != "" {
			cands = append(cands, filepath.Join(v, "KingsIsle Entertainment", "Wizard101"))
		}
	}
	cands = append(cands, `C:\KingsIsle Entertainment\Wizard101`, `C:\ProgramData\KingsIsle Entertainment\Wizard101`)
	for _, c := range cands {
		if isGameFolder(c) {
			return c
		}
	}
	return ""
}

// ---- Is this the live game? ----

func resolve(host string) []string {
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil
	}
	return addrs
}

func isPrivateIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	// Tailscale / carrier-grade NAT range 100.64.0.0/10
	_, cg, _ := net.ParseCIDR("100.64.0.0/10")
	return cg.Contains(ip)
}

// isLiveRealm is true when the address is the live game: one of its domains, or an address one of its servers uses.
func isLiveRealm(host string) bool {
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, d := range liveDomains {
		if name == d || strings.HasSuffix(name, "."+d) {
			return true
		}
	}
	addrs := resolve(name)
	if len(addrs) == 0 {
		return false
	}
	allPrivate := true
	for _, a := range addrs {
		if !isPrivateIP(a) {
			allPrivate = false
		}
	}
	if allPrivate {
		return false
	}
	for _, live := range liveHosts {
		for _, la := range resolve(live) {
			for _, a := range addrs {
				if a == la {
					return true
				}
			}
		}
	}
	return false
}

// ---- Checking the server and starting the game ----

func ping(host string, port int) (bool, int) {
	start := time.Now()
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 3*time.Second)
	if err != nil {
		return false, 0
	}
	_ = c.Close()
	return true, int(time.Since(start).Milliseconds())
}

func launchGame(folder string, p Profile) error {
	if !isGameFolder(folder) {
		return errors.New("Wizard101 wasn't found. Choose the folder that holds Bin\\WizardGraphicalClient.exe in Settings.")
	}
	if isLiveRealm(p.Address) {
		return errors.New("That's the live Wizard101 game. This launcher only plays on private servers.")
	}
	exe := clientPath(folder)
	cmd := exec.Command(exe, "-L", p.Address, strconv.Itoa(p.Port))
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Windows wouldn't start the game: %v", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openURL opens an http(s) address in the person's own browser.
func openURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("only web addresses can be opened")
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
