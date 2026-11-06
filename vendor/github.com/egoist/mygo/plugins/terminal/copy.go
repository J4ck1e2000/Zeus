package terminal

// This file is a local patch of the vendored plugin, with scroll.go: it
// adds copy on select, its OnCopy event, and the setters the settings
// apply to running sessions. A `go mod vendor` drops both files; restore
// them after regenerating the vendor directory.

// copyOnSelect copies the selection to the clipboard when the terminal
// copies on select, as the pointer lets the selection go. t.mu is held.
func (v *view) copyOnSelect() {
	t := v.t
	if !t.opts.CopyOnSelect || t.term == nil {
		return
	}
	text, ok := t.term.SelectionText()
	if !ok || text == "" {
		return
	}
	v.services.WriteClipboard(text)
	if t.opts.OnCopy != nil {
		t.opts.OnCopy()
	}
}

// SetScrollback changes how much output above the screen the terminal
// keeps, as the settings change while sessions run.
func (t *Terminal) SetScrollback(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return
	}
	t.term.SetScrollback(n)
}

// SetCursor changes the cursor's shape and whether it blinks, which
// programs may still change.
func (t *Terminal) SetCursor(s CursorStyle, blink bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return
	}
	t.term.SetDefaultCursor(vtCursor(s), blink)
}

// SetCopyOnSelect sets whether the selection copies as the pointer lets
// it go.
func (t *Terminal) SetCopyOnSelect(v bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.opts.CopyOnSelect = v
}
