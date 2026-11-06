package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// findState is the search of a pane's screen and scrollback: the query,
// the rows of the terminal when it last ran, and the rows that match.
type findState struct {
	open      bool
	pane      *Pane
	query     string
	rows      []string
	matches   []int
	sel       int
	wasBottom bool
}

// openFind shows the find bar at the focused pane, or hides it when it
// is already there.
func (a *App) openFind() {
	t := a.tab()
	if t == nil || t.Focus == nil {
		return
	}
	if a.find.open && a.find.pane == t.Focus {
		a.closeFind()
		return
	}
	a.find.open = true
	a.find.pane = t.Focus
	a.find.wasBottom = t.Focus.term != nil && t.Focus.term.AtBottom()
	a.focusReq = nil // the field takes the focus
	a.research()
}

// closeFind hides the find bar, gives the keys back to the pane, and
// returns to the bottom of the scrollback when the search started there.
func (a *App) closeFind() {
	a.find.open = false
	if p := a.find.pane; p != nil {
		if a.find.wasBottom && p.term != nil {
			total, _, length := p.term.ScrollState()
			p.term.ScrollToRow(max(0, total-length))
		}
		a.focusReq = p
	}
}

// research finds the query's rows in the pane's screen and scrollback:
// the terminal gives its rows whole, leaving out the blank ones at the
// foot of the screen, so the search anchors on the last row with text.
// It goes to the bottom of the scrollback and finds that row there,
// which tells where its rows sit.
func (a *App) research() {
	f := &a.find
	f.matches = f.matches[:0]
	p := f.pane
	q := strings.ToLower(strings.TrimSpace(f.query))
	if p == nil || p.term == nil || q == "" {
		return
	}
	rows := p.term.Rows()
	total, _, length := p.term.ScrollState()
	if len(rows) == 0 || length == 0 {
		return
	}
	p.term.ScrollToBottom()
	last := 0
	for y := length - 1; y >= 0; y-- {
		if strings.TrimSpace(p.term.RowText(y)) != "" {
			last = y
			break
		}
	}
	base := total - length + last - (len(rows) - 1)
	for i, r := range rows {
		if strings.Contains(strings.ToLower(r), q) {
			f.matches = append(f.matches, base+i)
		}
	}
	f.rows = rows
	f.sel = min(f.sel, max(len(f.matches)-1, 0))
}

// findShow brings the selected match into view, at the bottom of the
// viewport going down and at its top going up.
func (a *App) findShow(down bool) {
	f := &a.find
	p := f.pane
	if p == nil || p.term == nil || len(f.matches) == 0 {
		return
	}
	row := f.matches[f.sel]
	_, _, length := p.term.ScrollState()
	if down {
		p.term.ScrollToRow(max(0, row-length+1))
	} else {
		p.term.ScrollToRow(max(0, row))
	}
}

// findNext selects the next or the previous match and shows it, the
// search having run again, in case output moved the rows.
func (a *App) findNext(down bool) {
	f := &a.find
	if f.pane == nil {
		return
	}
	a.research()
	if len(f.matches) == 0 {
		return
	}
	if down {
		f.sel = (f.sel + 1) % len(f.matches)
	} else {
		f.sel = (f.sel - 1 + len(f.matches)) % len(f.matches)
	}
	a.findShow(down)
}

// findBar docks the search at the bottom of the focused pane's card.
func (a *App) findBar(c *ui.Context, k *colors) {
	f := &a.find
	// The search floats over the pane's corner in a pill of glass, as
	// the copied toast does, leaving the last rows of the screen alone.
	bar := ui.Row(c).Absolute().Right(12).Bottom(12).Height(38).Padding(0, 5, 0, 12).Gap(7).AlignItems(ui.Center).
		Radius(19).Material(panelGlass(k)).Border(1, k.panelBorder).
		Shadow(0, 2, 10, 0, k.shadowFocused)
	bar.Children(func() {
		ui.Icon(c, icon("search")).Size(14, 14).TextColor(k.textFaint)
		in := ui.TextInput(c, &f.query).Placeholder(tr("Find…")).Width(200).FontSize(12.5).AutoFocus()
		if in.Changed() {
			f.sel = 0
			a.research()
			a.findShow(true)
		}
		if in.Submitted() {
			a.findNext(true)
		}
		if in.Shortcut(ui.Shift, ui.KeyEnter) {
			a.findNext(false)
		}
		if in.Shortcut(0, ui.KeyEscape) {
			a.closeFind()
		}
		count := ""
		switch {
		case strings.TrimSpace(f.query) == "":
		case len(f.matches) == 0:
			count = tr("No matches")
		default:
			count = fmt.Sprintf(tr("%d of %d"), f.sel+1, len(f.matches))
		}
		ui.Text(c, count).FontSize(11.5).TextColor(k.textFaint).Shrink(0)
		if iconButton(c, k, "arrow-up", tr("Previous Match"), 24, 14).Clicked() {
			a.findNext(false)
		}
		if iconButton(c, k, "arrow-down", tr("Next Match"), 24, 14).Clicked() {
			a.findNext(true)
		}
		if iconButton(c, k, "x", tr("Close Find"), 24, 14).Clicked() {
			a.closeFind()
		}
	})
}
