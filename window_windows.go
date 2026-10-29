//go:build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
)

// openWindow shows the launcher in its own window. It returns false if WebView2 isn't available.
func openWindow(pageURL string, a *app) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	data := filepath.Join(configDir(), "webview")
	_ = os.MkdirAll(data, 0o755)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  data,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Spiral Navigator",
			Width:  1080,
			Height: 700,
			IconId: 1,
			Center: true,
		},
	})
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.SetSize(1080, 700, webview2.HintNone)
	w.Navigate(pageURL)
	go func() {
		<-a.quit
		w.Terminate()
	}()
	w.Run()
	return true
}

func showFatal(msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	mb := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString("Spiral Navigator")
	m, _ := syscall.UTF16PtrFromString(msg)
	mb.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
