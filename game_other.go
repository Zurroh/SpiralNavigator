//go:build !windows

package main

func registryInstallPaths() []string { return nil }

func browseFolder(start string) string { return "" }
