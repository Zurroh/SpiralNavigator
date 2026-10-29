package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanPatchPath(t *testing.T) {
	good := []string{"Data/GameData/Custom-WorldData.wad", "Bin/Config/thing.xml", "Data/Locale/en-US/Custom.lang"}
	bad := []string{"", "/etc/passwd", "../Data/x.wad", "Data/../Bin/x.wad", "Data/x.exe", "Bin/WizardGraphicalClient.exe",
		"Data\\x.wad", "C:/x.wad", "Data/x.dll", "Data/run.bat", "Data/x.ps1", "Other/x.wad", "Data//x.wad", "Data/x.wad.", "Data/a.WAD.exe", "Data/ x.exe "}
	for _, g := range good {
		if _, err := cleanPatchPath(g); err != nil {
			t.Errorf("%q should be allowed: %v", g, err)
		}
	}
	for _, b := range bad {
		if _, err := cleanPatchPath(b); err == nil {
			t.Errorf("%q should be refused", b)
		}
	}
}

func TestSafeJoin(t *testing.T) {
	root := t.TempDir()
	if _, err := safeJoin(root, "Data/x.wad"); err != nil {
		t.Fatal(err)
	}
	if _, err := safeJoin(root, "../x.wad"); err == nil {
		t.Fatal("escape allowed")
	}
}

func TestLiveRealm(t *testing.T) {
	for _, h := range []string{"login.us.wizard101.com", "WIZARD101.com", "patch.eu.wizard101.com.", "foo.kingsisle.com"} {
		if !isLiveRealm(h) {
			t.Errorf("%s should count as live", h)
		}
	}
	for _, h := range []string{"100.64.0.5", "127.0.0.1", "192.168.1.5", "localhost"} {
		if isLiveRealm(h) {
			t.Errorf("%s should not count as live", h)
		}
	}
}

func TestShareRoundTrip(t *testing.T) {
	p := Profile{Name: "Realm", Address: "play.example.com", Port: 12000, NotesURL: "https://example.com/n.md", PatchURL: "https://example.com/p/", School: "fire", BugURL: "https://example.com/bugs", GameFolder: `C:\secret`, AutoPatch: true}
	got, err := parseShared(shareCode(p))
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != p.Name || got.Address != p.Address || got.Port != p.Port || got.NotesURL != p.NotesURL || got.PatchURL != p.PatchURL || got.BugURL != p.BugURL || got.School != "fire" {
		t.Errorf("round trip lost data: %+v", got)
	}
	if got.GameFolder != "" || got.AutoPatch {
		t.Error("a shared server must not bring a game folder or auto-patching with it")
	}
	if _, err := parseShared("w101srv:@@@"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestCleanProfile(t *testing.T) {
	ok := Profile{Name: "A", Address: "100.64.0.5", Port: 12000, School: "storm"}
	if err := cleanProfile(&ok); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Profile{
		{Name: "", Address: "a.b", Port: 1},
		{Name: "x", Address: "bad host", Port: 1},
		{Name: "x", Address: "a;b", Port: 1},
		{Name: "x", Address: "a.b", Port: 0},
		{Name: "x", Address: "a.b", Port: 70000},
		{Name: "x", Address: "a.b", Port: 1, NotesURL: "ftp://x/y"},
		{Name: "x", Address: "a.b", Port: 1, NotesURL: "javascript:alert(1)"},
	} {
		b := bad
		if err := cleanProfile(&b); err == nil {
			t.Errorf("%+v should be refused", bad)
		}
	}
	p := Profile{Name: "x", Address: "a.b", Port: 1, NotesURL: "example.com/patchnotes.md"}
	if err := cleanProfile(&p); err != nil || !strings.HasPrefix(p.NotesURL, "https://") {
		t.Errorf("scheme-less notes address: %v %q", err, p.NotesURL)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "Data", "GameData"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "Data", "GameData", "a.wad"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "evil.exe"), []byte("x"), 0o644)
	if err := makeManifest(dir, "1"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if strings.Contains(string(b), "evil.exe") || !strings.Contains(string(b), "Data/GameData/a.wad") {
		t.Errorf("manifest wrong: %s", b)
	}
}
