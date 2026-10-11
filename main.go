// super_v_clipboard is a small clipboard history for Linux.
//
// Run with no arguments it starts the background daemon, which watches the
// clipboard and owns the history window. Run as "super_v_clipboard show" it
// asks the running daemon to open that window (bind this to Super+V).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"golang.design/x/clipboard"
)

const (
	appName    = "super_v_clipboard"
	maxEntries = 50      // how many clipboard entries to remember
	maxBytes   = 1 << 20 // skip anything bigger than 1 MB (huge logs, etc.)
)

// ---------- history ----------

// History is the list of copied text, newest first, saved to disk on every change.
type History struct {
	mu      sync.Mutex
	entries []string
	path    string
}

// loadHistory reads ~/.local/share/super_v_clipboard/history.json, or starts empty.
func loadHistory() *History {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", appName)
	_ = os.MkdirAll(dir, 0o700)
	h := &History{path: filepath.Join(dir, "history.json")}
	if data, err := os.ReadFile(h.path); err == nil {
		_ = json.Unmarshal(data, &h.entries)
	}
	return h
}

// Add puts s at the top of the history. Blank or very large text is ignored,
// and if s is already in the list it moves to the top instead of appearing twice.
func (h *History) Add(s string) {
	if len(s) > maxBytes || strings.TrimSpace(s) == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) > 0 && h.entries[0] == s {
		return
	}
	for i, e := range h.entries {
		if e == s {
			h.entries = append(h.entries[:i], h.entries[i+1:]...)
			break
		}
	}
	h.entries = append([]string{s}, h.entries...)
	if len(h.entries) > maxEntries {
		h.entries = h.entries[:maxEntries]
	}
	h.saveLocked()
}

// Clear deletes every entry, on disk too.
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = nil
	h.saveLocked()
}

// Snapshot returns a copy of the entries that is safe to use without the lock.
func (h *History) Snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.entries...)
}

// saveLocked writes the history to disk. The caller must hold h.mu.
func (h *History) saveLocked() {
	data, _ := json.Marshal(h.entries)
	if err := os.WriteFile(h.path, data, 0o600); err != nil { // 0600: clipboards can hold passwords
		log.Println("could not save history:", err)
	}
}

// ---------- helpers ----------

// socketPath is where the daemon listens for "show" requests.
func socketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, appName+".sock")
}

// preview turns an entry into one short line for the list.
func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ") // collapse newlines and tabs
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// keyList is a widget.List that also selects on Enter and hides on Escape.
// The stock list only selects on Space and ignores other keys.
type keyList struct {
	widget.List
	onEscape func()
}

func (l *keyList) TypedKey(k *fyne.KeyEvent) {
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		l.List.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	case fyne.KeyEscape:
		l.onEscape()
	default:
		l.List.TypedKey(k)
	}
}

// ---------- main ----------

func main() {
	sock := socketPath()

	if len(os.Args) > 1 {
		if os.Args[1] != "show" {
			fmt.Fprintf(os.Stderr, "usage: %s [show]\n\n", appName)
			fmt.Fprintln(os.Stderr, "  (no argument)  start the clipboard daemon")
			fmt.Fprintln(os.Stderr, "  show           open the history window")
			os.Exit(2)
		}
		// Ask the running daemon to show its window.
		conn, err := net.Dial("unix", sock)
		if err != nil {
			fmt.Fprintln(os.Stderr, appName, "daemon is not running:", err)
			os.Exit(1)
		}
		conn.Close()
		return
	}

	// Only one daemon at a time: if something answers on the socket, quit.
	if c, err := net.Dial("unix", sock); err == nil {
		c.Close()
		log.Fatal(appName, " is already running")
	}
	runDaemon(sock)
}

// runDaemon watches the clipboard, serves the history window and runs until logout.
func runDaemon(sock string) {
	if err := clipboard.Init(); err != nil {
		log.Fatal("clipboard init failed: ", err)
	}
	hist := loadHistory()

	// Without a virtual keyboard we can still copy, just not paste for the user.
	kb, err := openKeyboard()
	if err != nil {
		log.Println("auto-paste disabled (no access to /dev/uinput):", err)
	}

	a := app.NewWithID("io.github.naftulisinger.super_v_clipboard")
	w := a.NewWindow("Clipboard history")
	w.Resize(fyne.NewSize(460, 520))
	w.SetFixedSize(true)
	w.CenterOnScreen()
	w.SetCloseIntercept(func() { w.Hide() }) // closing just hides, the daemon keeps running

	// items is what the list shows. It is only touched on the Fyne UI thread.
	var items []string

	list := &keyList{onEscape: w.Hide}
	list.Length = func() int { return len(items) }
	list.CreateItem = func() fyne.CanvasObject {
		l := widget.NewLabel("")
		l.Truncation = fyne.TextTruncateEllipsis
		return l
	}
	list.UpdateItem = func(id widget.ListItemID, o fyne.CanvasObject) {
		o.(*widget.Label).SetText(preview(items[id]))
	}
	list.ExtendBaseWidget(list)

	// Picking an entry: copy it, hide the window, then press Ctrl+V for the user.
	list.OnSelected = func(id widget.ListItemID) {
		clipboard.Write(clipboard.FmtText, []byte(items[id]))
		list.UnselectAll()
		w.Hide()
		if kb != nil {
			go func() {
				time.Sleep(200 * time.Millisecond) // let focus return to the previous window
				kb.PasteShortcut()
			}()
		}
	}

	clearBtn := widget.NewButton("Clear history", func() {
		hist.Clear()
		items = nil
		list.Refresh()
	})

	w.SetContent(container.NewBorder(nil, clearBtn, nil, nil, list))
	// Escape also works when the list doesn't have focus (e.g. after clicking the button).
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyEscape {
			w.Hide()
		}
	})

	show := func() {
		items = hist.Snapshot()
		list.Refresh()
		list.ScrollToTop()
		w.Show()
		w.RequestFocus()
		w.Canvas().Focus(list) // so arrow keys and Enter work right away
	}

	// Tray icon (GNOME needs the AppIndicator extension; Ubuntu ships it).
	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayMenu(fyne.NewMenu("Super V Clipboard",
			fyne.NewMenuItem("Show history", show),
		))
	}

	// Save every new piece of copied text.
	go func() {
		for data := range clipboard.Watch(context.Background(), clipboard.FmtText) {
			hist.Add(string(data))
		}
	}()

	// Any connection to the socket means "show the window".
	_ = os.Remove(sock) // remove a stale socket left by a crash
	ln, err := net.Listen("unix", sock)
	if err != nil {
		log.Fatal("cannot open socket: ", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
			fyne.Do(show) // UI calls must run on the Fyne thread
		}
	}()

	a.Run()
}
