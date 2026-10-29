//go:build !windows

package main

import "fmt"

func openWindow(pageURL string, a *app) bool { return false }

func showFatal(msg string) { fmt.Println(msg) }
