package terminal

import "strings"

// This file is a local patch of the vendored plugin: it exposes the
// emulator's scroll state, which Zeus's find bar searches with. A
// `go mod vendor` drops it; restore it after regenerating the vendor
// directory.

// AtBottom reports whether the viewport follows the active screen,
// rather than showing the scrollback.
func (t *Terminal) AtBottom() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return true
	}
	return t.term.AtBottom()
}

// ScrollState returns the rows of the screen and its scrollback (total),
// the row the viewport shows first (top), and how many rows it shows
// (length).
func (t *Terminal) ScrollState() (total, top, length int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return 0, 0, 0
	}
	sb := t.term.Scrollbar()
	return int(sb.Total), int(sb.Offset), int(sb.Len)
}

// ScrollToBottom moves the viewport to the active screen.
func (t *Terminal) ScrollToBottom() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return
	}
	t.term.ScrollToBottom()
}

// ScrollToRow moves the viewport to show row first, of the screen and
// its scrollback.
func (t *Terminal) ScrollToRow(row int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return
	}
	t.term.ScrollToRow(row)
}

// Rows returns the text of the screen's rows, leaving out the blank ones
// at its foot.
func (t *Terminal) Rows() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil
	}
	_, vh := t.term.Size()
	rows := make([]string, 0, vh)
	for y := 0; y < vh; y++ {
		text, _ := t.term.RowText(y)
		line := strings.TrimRight(string(text), " ")
		if line == "" {
			continue
		}
		rows = append(rows, line)
	}
	return rows
}

// RowText returns the text of row y of the viewport, of the screen and
// its scrollback.
func (t *Terminal) RowText(y int) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return ""
	}
	text, _ := t.term.RowText(y)
	return strings.TrimRight(string(text), " ")
}
