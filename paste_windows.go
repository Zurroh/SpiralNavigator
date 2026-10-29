//go:build windows && amd64

package main

import (
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// Paste into the game's chat. Wizard101's chat box takes no Ctrl+V, so while the launcher is open, Ctrl+Shift+V
// while the game is the active window types the clipboard into it, the way a keyboard would:
//   - one line: typed into the chat box you already have open (you press Enter);
//   - several lines: each line is sent as its own message (chat must be closed; Enter opens it).
// It only acts when the active window's title is "Wizard101", and it reads keys with GetAsyncKeyState instead of
// registering a hotkey, so Ctrl+Shift+V keeps working in every other program.

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	kernel32   = syscall.NewLazyDLL("kernel32.dll")
	pKeyState  = user32.NewProc("GetAsyncKeyState")
	pForeWin   = user32.NewProc("GetForegroundWindow")
	pWinText   = user32.NewProc("GetWindowTextW")
	pSendInput = user32.NewProc("SendInput")
	pOpenClip  = user32.NewProc("OpenClipboard")
	pCloseClip = user32.NewProc("CloseClipboard")
	pGetClip   = user32.NewProc("GetClipboardData")
	pGlobalLk  = kernel32.NewProc("GlobalLock")
	pGlobalUnl = kernel32.NewProc("GlobalUnlock")
)

const (
	vkReturn  = 0x0D
	vkShift   = 0x10
	vkControl = 0x11
	vkV       = 0x56

	cfUnicodeText = 13

	inputKeyboard   = 1
	keyeventUnicode = 0x0004
	keyeventKeyUp   = 0x0002
)

// INPUT for a keyboard event on 64-bit Windows (40 bytes).
type keyInput struct {
	typ       uint32
	_         uint32
	vk        uint16
	scan      uint16
	flags     uint32
	time      uint32
	_         uint32
	extraInfo uintptr
	_         [8]byte
}

var pasting atomic.Bool

func startPasteHelper() {
	go func() {
		wasDown := false
		for {
			time.Sleep(40 * time.Millisecond)
			down := keyDown(vkControl) && keyDown(vkShift) && keyDown(vkV)
			if down && !wasDown && gameIsActive() && pasting.CompareAndSwap(false, true) {
				go func() {
					defer pasting.Store(false)
					pasteClipboard()
				}()
			}
			wasDown = down
		}
	}()
}

func keyDown(vk int) bool {
	r, _, _ := pKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

func gameIsActive() bool {
	h, _, _ := pForeWin.Call()
	if h == 0 {
		return false
	}
	buf := make([]uint16, 128)
	n, _, _ := pWinText.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.HasPrefix(syscall.UTF16ToString(buf[:n]), "Wizard101")
}

func clipboardText() string {
	if r, _, _ := pOpenClip.Call(0); r == 0 {
		return ""
	}
	defer pCloseClip.Call()
	h, _, _ := pGetClip.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := pGlobalLk.Call(h)
	if p == 0 {
		return ""
	}
	defer pGlobalUnl.Call(h)
	var units []uint16
	for i := uintptr(0); i < 1<<16; i++ {
		u := *(*uint16)(unsafe.Pointer(p + i*2))
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}

func sendKey(vk uint16, scan uint16, flags uint32) {
	in := keyInput{typ: inputKeyboard, vk: vk, scan: scan, flags: flags}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func pressEnter() {
	sendKey(vkReturn, 0, 0)
	sendKey(vkReturn, 0, keyeventKeyUp)
}

func typeText(s string) {
	for _, u := range utf16.Encode([]rune(s)) {
		sendKey(0, u, keyeventUnicode)
		sendKey(0, u, keyeventUnicode|keyeventKeyUp)
		time.Sleep(12 * time.Millisecond)
	}
}

func pasteClipboard() {
	// Let go of Ctrl+Shift first, or the game sees them held down while the text arrives.
	for i := 0; i < 100 && (keyDown(vkControl) || keyDown(vkShift)); i++ {
		time.Sleep(30 * time.Millisecond)
	}
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(clipboardText(), "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	switch len(lines) {
	case 0:
		return
	case 1:
		typeText(lines[0])
		return
	}
	for _, l := range lines {
		if !gameIsActive() {
			return
		}
		pressEnter() // opens the chat box
		time.Sleep(200 * time.Millisecond)
		typeText(l)
		time.Sleep(150 * time.Millisecond)
		pressEnter() // sends it
		time.Sleep(600 * time.Millisecond)
	}
}
