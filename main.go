package main

import (
	_ "embed"
	"fmt"
	"os"
	"time"
)

const version = "1.0.0"

//go:embed defaults.json
var defaultProfiles []byte

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "make-manifest":
			if len(os.Args) < 3 {
				fmt.Println("Usage: launcher make-manifest <folder> [version]\nThe folder should hold files laid out like your game folder, e.g. Data/GameData/MyServer.wad")
				os.Exit(2)
			}
			ver := ""
			if len(os.Args) > 3 {
				ver = os.Args[3]
			}
			if err := makeManifest(os.Args[2], ver); err != nil {
				fmt.Println("Failed:", err)
				os.Exit(1)
			}
			return
		case "--version":
			fmt.Println(version)
			return
		}
	}

	st, err := loadStore(defaultProfiles)
	if err != nil {
		showFatal("The launcher couldn't read its settings: " + err.Error())
		os.Exit(1)
	}
	a := newApp(st)
	startPasteHelper()
	pageURL, err := a.serve()
	if err != nil {
		showFatal("The launcher couldn't start: " + err.Error())
		os.Exit(1)
	}

	// --serve: run for a browser only (for testing and for computers with no WebView2).
	for _, arg := range os.Args[1:] {
		if arg == "--serve" {
			fmt.Println(pageURL)
			<-a.quit
			return
		}
	}

	if openWindow(pageURL, a) {
		return
	}
	// No embedded window is possible here (WebView2 missing): use the default browser, and quit when the tab closes.
	if err := openURL(pageURL); err != nil {
		showFatal("The launcher couldn't open a window. Install the WebView2 runtime from Microsoft, then try again.")
		return
	}
	for {
		select {
		case <-a.quit:
			return
		case <-time.After(5 * time.Second):
			if a.beaten.Load() && time.Now().Unix()-a.lastBeat.Load() > 20 {
				return
			}
		}
	}
}
