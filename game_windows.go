//go:build windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func registryInstallPaths() []string {
	var out []string
	for _, p := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\KingsIsle Entertainment\Wizard101`},
		{registry.LOCAL_MACHINE, `SOFTWARE\KingsIsle Entertainment\Wizard101`},
		{registry.CURRENT_USER, `SOFTWARE\KingsIsle Entertainment\Wizard101`},
	} {
		k, err := registry.OpenKey(p.root, p.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		if v, _, err := k.GetStringValue("InstallPath"); err == nil && v != "" {
			out = append(out, v)
		}
		k.Close()
	}
	return out
}

// browseFolder shows Windows' own folder picker. It returns "" when the person cancels.
func browseFolder(start string) string {
	script := `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Choose your Wizard101 folder (the one that contains Bin)'
$d.ShowNewFolderButton = $false
if ($env:W101_START) { $d.SelectedPath = $env:W101_START }
if ($d.ShowDialog() -eq 'OK') { [Console]::Out.Write($d.SelectedPath) }`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), "W101_START="+start)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
