package main

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

const (
	gap        = 8  // between panes, and around them
	titleH     = 44 // the title bar
	cardR      = 16 // the panes' corners
	headerH    = 33 // the panes' headers
	activeFor  = 1500 * time.Millisecond
	divHit     = 4                      // how far the divider's hit area reaches into the panes
	copiedHold = 900 * time.Millisecond // how long the copied toast stays
	copiedFade = 240 * time.Millisecond // how long it takes to leave
)

func (a *App) view(c *ui.Context) {
	a.services = c.Services()
	a.runPosted()
	k := colorsOf(c)
	a.focusedWin = a.win == nil || a.win.IsFocused()
	c.Root().Background(k.bgBottom)
	ui.Column(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { paintBackground(p, r, k) }).Children(func() {
		a.titleBar(c, k)
		ui.Row(c).Grow(1).MinHeight(0).Padding(0, gap, gap, gap).Gap(gap).AlignItems(ui.Stretch).Children(func() {
			if a.fileTree.open {
				a.fileTreePanel(c, k)
			}
			if t := a.tab(); t != nil {
				ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					a.tabContent(c, k, t)
				})
			}
		})
	})
	if a.err != "" {
		a.errorBanner(c, k)
	}
	a.shortcuts(c)
	a.palette(c, k)
	if a.saveDue && time.Since(a.lastSave) > time.Second {
		a.save()
	}
	// The window's title, which the Window menu and Mission Control show,
	// is the active tab's.
	if t := a.tab(); t != nil && a.win != nil {
		name, detail := t.label()
		if title := strings.TrimSpace(name + " " + detail); title != a.title {
			a.title = title
			a.win.SetTitle(title)
		}
	}
	a.syncFileTree()
}

// ownControls tells that the title bar draws the window's controls: on
// Windows, whose own would look like none of the rest of it.
var ownControls = runtime.GOOS == "windows"

// titleBar draws the title bar under the window's controls: the host,
// the tabs, and the buttons of the command palette and a new tab.
func (a *App) titleBar(c *ui.Context, k *colors) {
	bar := c.TitleBar()
	left := bar.Left
	if left == 0 {
		left = 12 // in full screen, or beside no controls
	} else {
		left += 14
	}
	ui.Row(c).Height(titleH).Padding(0, max(bar.Right, 10), 0, left).Gap(14).AlignItems(ui.Center).DragWindow().Children(func() {
		a.hostChip(c, k)
		a.tabStrip(c, k)
		// The title bar between the tabs and the buttons, which drags the
		// window, and a double click on which zooms it.
		ui.Spacer(c).MinWidth(titleFree - 14)
		ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
			if chromeButton(c, k, "folder-open", tr("Toggle File Tree"), 30, 16).Tooltip(tr("Toggle File Tree")).Clicked() {
				a.toggleFileTree()
			}
			if chromeButton(c, k, "command", tr("Command Palette"), 30, 17).Tooltip(tip(tr("Command Palette"), &cmdPalette)).Clicked() {
				a.openPalette()
			}
			if chromeButton(c, k, "plus", tr("New Tab"), 30, 19).Tooltip(tip(tr("New Tab"), &cmdNewTab)).Clicked() {
				a.newTab(a.newTabDir())
			}
		})
		a.windowControls(c, k)
	})
}

// windowControls draws the buttons that minimize, maximize and close the
// window, when it has none of its own: they are the title bar's other
// buttons, and close's face turns red, as Windows has it.
func (a *App) windowControls(c *ui.Context, k *colors) {
	a.windowControlsFor(c, k, a.win)
}

func (a *App) windowControlsFor(c *ui.Context, k *colors, win *mygo.Window) {
	if !ownControls || win == nil || win.IsFullScreen() {
		return
	}
	maximized := win.IsMaximized()
	ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Box(c).Size(1, 16).Margin(0, 6).Background(k.tabSep)
		if iconButton(c, k, "minus", tr("Minimize"), 30, 16).Tooltip(tr("Minimize")).Clicked() {
			win.Minimize()
		}
		glyph, label := "square", tr("Maximize")
		if maximized {
			glyph, label = "copy", tr("Restore")
		}
		if iconButton(c, k, glyph, label, 30, 13).Tooltip(label).Clicked() {
			if maximized {
				win.Unmaximize()
			} else {
				win.Maximize()
			}
		}
		b := ui.Box(c).Size(30, 30).Center().Radius(30 / 2.6).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(tr("Close")).Tooltip(tr("Close"))
		col := k.iconMuted
		if b.Pressed() {
			b.Background(closeRed.Alpha(0.8))
			col = ui.Hex("#ffffff")
		} else if b.Hovered() {
			b.Background(closeRed)
			col = ui.Hex("#ffffff")
		}
		b.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
		b.Children(func() {
			ui.Icon(c, icon("x")).Size(17, 17).TextColor(col)
		})
		if b.Clicked() {
			win.Close()
		}
	})
}

// closeRed is the face of the close button under the pointer: Windows 11's.
var closeRed = ui.Hex("#c42b1c")

// tip is the tooltip of a button that runs a command: its label and its
// keys.
func tip(label string, cmd *command) string {
	keys := cmd.keysOf()
	if keys == "" {
		return label
	}
	return label + "  " + keys
}

// chromeButton is a button of the title bar: a circular face of glass,
// which reads over the window's gradient, with a face on hover. Label
// names it for screen readers and its tooltip.
func chromeButton(c *ui.Context, k *colors, name, label string, size, iconSize float32) ui.Element {
	b := ui.Box(c).Size(size, size).Center().Radius(size/2).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(label).
		Material(glass.Glass{Tint: k.chromeTint}).Border(0.5, k.panelBorder)
	if b.Pressed() {
		b.Background(k.pressed)
	} else if b.Hovered() {
		b.Background(k.hover)
	}
	b.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	b.Children(func() {
		col := k.iconMuted
		if b.Hovered() {
			col = k.text
		}
		ui.Icon(c, icon(name)).Size(iconSize, iconSize).TextColor(col)
	})
	return b
}

// iconButton is a borderless button of an icon, with a face on hover;
// label names it for screen readers and its tooltip.
func iconButton(c *ui.Context, k *colors, name, label string, size, iconSize float32) ui.Element {
	b := ui.Box(c).Size(size, size).Center().Radius(size / 2.6).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(label)
	if b.Pressed() {
		b.Background(k.pressed)
	} else if b.Hovered() {
		b.Background(k.hover)
	}
	b.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	col := k.iconMuted
	if b.Hovered() {
		col = k.text
	}
	b.Children(func() {
		ui.Icon(c, icon(name)).Size(iconSize, iconSize).TextColor(col)
	})
	return b
}

// tabContent lays out the panes of a tab, or the one zoomed.
func (a *App) tabContent(c *ui.Context, k *colors, t *Tab) {
	if t.Zoom != nil {
		a.paneCard(c, k, t, t.Zoom).Grow(1)
		return
	}
	a.node(c, k, t, t.Root).Grow(1)
}

// node lays out a node of the tree of splits.
func (a *App) node(c *ui.Context, k *colors, t *Tab, n *Node) ui.Element {
	if n.Pane != nil {
		return a.paneCard(c, k, t, n.Pane)
	}
	var box ui.Element
	if n.Vertical {
		box = ui.Column(c.Key(n.ID))
	} else {
		box = ui.Row(c.Key(n.ID))
	}
	box.MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
	enter := &ui.Motion{Opacity: 0}
	if n.Vertical {
		enter.Y = 6
	} else {
		enter.X = 6
	}
	box.Transition(ui.ElementTransition{
		Colors: true, Duration: 160 * time.Millisecond, Enter: enter,
	})
	bounds := box.Bounds()
	// The tree may change as its panes build, as a split: build the
	// children it has now.
	first, second, ratio := n.A, n.B, n.Ratio
	box.Children(func() {
		a.node(c, k, t, first).Grow(ratio).Basis(0).MinWidth(0).MinHeight(0)
		div := ui.Box(c.Key("divider"))
		if n.Vertical {
			div.Height(gap)
		} else {
			div.Width(gap)
		}
		a.node(c, k, t, second).Grow(1 - ratio).Basis(0).MinWidth(0).MinHeight(0)
		// The hit area sits over the divider of the last frame and
		// reaches into the panes on both sides: the gap alone is a small
		// target to catch. Built after the panes, it takes their presses
		// there, and it takes the splitter's role and label for
		// accessibility as well. While it drags, the pressed element
		// keeps the moves however far it lags behind.
		d := n.divBounds
		hit := ui.Box(c.Key("divider hit")).Absolute().Role(ui.RoleSplitter).Label(tr("Divider"))
		if n.Vertical {
			hit.Left(d.X - bounds.X).Top(d.Y - bounds.Y - divHit).Width(d.W).Height(d.H + 2*divHit).
				Cursor(ui.CursorResizeRow)
		} else {
			hit.Left(d.X - bounds.X - divHit).Top(d.Y - bounds.Y).Width(d.W + 2*divHit).Height(d.H).
				Cursor(ui.CursorResizeColumn)
		}
		if dx, dy, ok := hit.Dragged(); ok {
			total, delta := bounds.W-gap, dx
			if n.Vertical {
				total, delta = bounds.H-gap, dy
			}
			if total > 0 {
				n.Ratio = min(max(n.Ratio+delta/total, 0.08), 0.92)
				a.changed()
			}
		}
		if hit.DoubleClicked() {
			n.Ratio = 0.5
			a.changed()
		}
		hovered := hit.Hovered() || hit.Dragging() || hit.Pressed()
		dragging := hit.Dragging() || hit.Pressed()
		n.divBounds = div.Bounds()
		if hovered {
			div.Draw(func(p *ui.Painter, r ui.Rect) {
				// While the divider drags, the handle grows to a long
				// accent line, which shows what the drag moves.
				col, length := k.iconMuted.Alpha(0.6), float32(36)
				if dragging {
					col, length = k.focus, 160
				}
				if n.Vertical {
					w := min(length, r.W-8)
					p.Fill(ui.Rect{X: r.X + r.W/2 - w/2, Y: r.Y + r.H/2 - 1.5, W: w, H: 3}, col, 1.5)
				} else {
					h := min(length, r.H-8)
					p.Fill(ui.Rect{X: r.X + r.W/2 - 1.5, Y: r.Y + r.H/2 - h/2, W: 3, H: h}, col, 1.5)
				}
			})
		}
	})
	return box
}

// paneCard draws a pane: a card with a header over its terminal.
func (a *App) paneCard(c *ui.Context, k *colors, t *Tab, p *Pane) ui.Element {
	focused := t.Focus == p
	zoomed := t.Zoom == p
	card := ui.Column(c.Key(p.ID)).Radius(cardR).Clip().MinWidth(0).MinHeight(0)
	border, shadow := k.cardBorder, k.shadow
	borderW := float32(1)
	if focused {
		border, shadow = k.cardBorderFocused, k.shadowFocused
		borderW = 1.5
	}
	if zoomed {
		// A zoomed pane carries the accent rim, which it keeps apart
		// from the focused pane's brighter rim of glass.
		border, borderW = k.focus, 1.5
	}
	card.Material(paneGlass(k, focused)).Border(borderW, border).
		Shadow(0, 1, 2, 0, shadow).
		Shadow(0, 6, 22, -2, shadow)
	card.Transition(ui.ElementTransition{
		Position: true, Size: true, Colors: true,
		Duration: 160 * time.Millisecond,
	})
	hovered := card.Hovered()
	card.Children(func() {
		a.paneHeader(c, k, t, p, focused, hovered)
		body := ui.Box(c).Grow(1).MinHeight(0).Padding(0, 5, 6, 5)
		body.Children(func() {
			if p.term == nil {
				return
			}
			tv := terminal.View(c, p.term).Fill()
			if a.focusReq == p && a.tab() == t {
				tv.Focus()
				a.focusReq = nil
			}
			if tv.Focused() && t.Focus != p {
				t.setFocus(p)
				a.changed()
			}
			if tv.Focused() {
				p.attention = false
			}
			if tv.Focused() && a.find.open && a.find.pane != p {
				a.find.pane = p // the search follows the focused pane
				a.research()
			}
		})
		if a.find.open && a.find.pane == p {
			a.findBar(c, k)
		}
		a.copiedToast(c, k, p)
	})
	p.bounds = card.Bounds()
	if card.Pressed() && t.Focus != p {
		t.setFocus(p)
		a.focusReq = p
	}
	return card
}

// copiedToast tells that the pane just put a selection on the clipboard:
// a pill in the card's corner, which rises in and, after a moment, sinks
// back out.
func (a *App) copiedToast(c *ui.Context, k *colors, p *Pane) {
	if p.copiedAt.IsZero() {
		return
	}
	since := time.Since(p.copiedAt)
	if since >= copiedHold {
		return // not built: the Exit motion carries it out
	}
	c.After(copiedHold - since) // the frame that stops building it, and its Exit begins
	pill := ui.Row(c.Key("copied toast")).Absolute().Right(12).Bottom(14).PassThrough().
		Gap(6).Padding(5, 10, 5, 8).Radius(20).
		Material(panelGlass(k)).Border(1, k.panelBorder).
		Shadow(0, 2, 10, 0, k.shadowFocused)
	pill.Transition(ui.ElementTransition{
		Duration: copiedFade,
		Enter:    &ui.Motion{Y: 10, Opacity: 0},
		Exit:     &ui.Motion{Y: 10, Opacity: 0},
	})
	pill.Children(func() {
		ui.Icon(c, icon("check")).Size(13, 13).TextColor(k.busy)
		ui.Text(c, tr("Copied")).FontSize(12).FontWeight(500).TextColor(k.text)
	})
}

// paneHeader draws a pane's program, title and directory, its activity,
// and its buttons.
func (a *App) paneHeader(c *ui.Context, k *colors, t *Tab, p *Pane, focused, hovered bool) {
	name, detail := p.label()
	prog := paneProgram(p)
	h := ui.Row(c).Height(headerH).Padding(0, 8, 0, 12).Gap(7).AlignItems(ui.Center).MinWidth(0).
		BorderWidth(0, 0, 0.5, 0).BorderColor(k.tabSep)
	if h.DoubleClicked() {
		t.setFocus(p)
		a.toggleZoom()
	}
	if h.Clicked() {
		t.setFocus(p)
		a.focusReq = p
	}
	h.Children(func() {
		ui.Icon(c, icon(prog.Glyph)).Size(14.5, 14.5).TextColor(k.text)
		ui.Row(c).Grow(1).MinWidth(0).Gap(5).AlignItems(ui.Center).ClipX().Children(func() {
			ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(k.text).SingleLine().Ellipsis("…").Shrink(0).MaxWidthPercent(80)
			if detail != "" {
				ui.Text(c, detail).FontSize(12.5).FontWeight(500).TextColor(k.text.Alpha(0.86)).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			}
			a.activityBadge(c, k, p)
		})
		show := focused || hovered
		ui.Row(c).Gap(1).AlignItems(ui.Center).Opacity(map[bool]float32{true: 1, false: 0}[show]).Children(func() {
			if !show {
				return
			}
			// The pane's buttons gather in a capsule of glass, which
			// reads over whatever the terminal shows.
			ui.Row(c).Padding(2, 3).Radius(13).Material(panelGlass(k)).Border(1, k.tabSep).AlignItems(ui.Center).Children(func() {
				if iconButton(c, k, "columns-2", tr("Split Right"), 22, 14).Tooltip(tip(tr("Split Right"), &cmdSplitRight)).Clicked() {
					t.setFocus(p)
					a.split(false)
				}
				if iconButton(c, k, "rows-2", tr("Split Down"), 22, 14).Tooltip(tip(tr("Split Down"), &cmdSplitDown)).Clicked() {
					t.setFocus(p)
					a.split(true)
				}
				zoom, label := "maximize-2", tr("Zoom")
				if t.Zoom == p {
					zoom, label = "minimize-2", tr("Unzoom")
				}
				if iconButton(c, k, zoom, label, 22, 14).Tooltip(tip(label, &cmdZoom)).Clicked() {
					t.setFocus(p)
					a.toggleZoom()
				}
				if iconButton(c, k, "x", tr("Close Pane"), 22, 14).Tooltip(tip(tr("Close Pane"), &cmdClosePane)).Clicked() {
					a.later(c, func() { a.closePane(p) })
				}
			})
		})
	})
}

// status is what a pane is doing.
type status int

const (
	statusIdle      status = iota // a shell at its prompt
	statusRunning                 // a program printing
	statusQuiet                   // a program running quietly
	statusAttention               // done or ringing out of sight
)

func (a *App) statusOf(c *ui.Context, p *Pane) status {
	if p.attention {
		return statusAttention
	}
	if p.info.Idle || p.info.Program == "" || programOf(p.info.Program).Shell {
		return statusIdle
	}
	last := time.Unix(0, p.lastData.Load())
	if since := c.Now().Sub(last); since < activeFor {
		c.After(activeFor - since)
		return statusRunning
	}
	return statusQuiet
}

// activityBadge shows what a pane does: a pulse while its program
// prints, a ring while it runs quietly, a dot when it asks for attention.
func (a *App) activityBadge(c *ui.Context, k *colors, p *Pane) {
	switch a.statusOf(c, p) {
	case statusRunning:
		activityDot(c, k.busy, 6)
	case statusQuiet:
		quietDot(c, k.busy, 5.5)
	case statusAttention:
		ui.Box(c).Size(7, 7).Radius(4).Background(k.attention).Margin(0, 0, 0, 2).Tooltip(tr("Needs attention"))
	}
}

// activityDot is the dot of a program printing: a solid dot in a soft
// halo. It does not animate, as drawing frames all along would cost more
// than it tells.
func activityDot(c *ui.Context, col ui.Color, size float32) ui.Element {
	e := ui.Box(c).Size(size+6, size+6).Margin(0, 0, 0, 1).Tooltip(tr("Printing"))
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		halo := size/2 + 2.5
		p.Fill(ui.Rect{X: cx - halo, Y: cy - halo, W: 2 * halo, H: 2 * halo}, col.Alpha(0.22), halo)
		p.Fill(ui.Rect{X: cx - size/2, Y: cy - size/2, W: size, H: size}, col, size/2)
	})
	return e
}

// quietDot is the mark of a program running without printing: a ring,
// quieter than the solid dot of one printing.
func quietDot(c *ui.Context, col ui.Color, size float32) ui.Element {
	e := ui.Box(c).Size(size+6, size+6).Margin(0, 0, 0, 1).Tooltip(tr("Running"))
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		p.Stroke(ui.Rect{X: cx - size/2, Y: cy - size/2, W: size, H: size}, col.Alpha(0.7), size/2, 1.5)
	})
	return e
}

// errorBanner shows what went wrong with the server.
func (a *App) errorBanner(c *ui.Context, k *colors) {
	ui.Overlay(c, func() {
		ui.Row(c).Absolute().Bottom(18).Left(0).Right(0).Justify(ui.Center).PassThrough().Children(func() {
			ui.Row(c).Gap(10).Padding(8, 10, 8, 14).Radius(14).Material(panelGlass(k)).Border(1, k.panelBorder).
				Shadow(0, 8, 24, 0, k.shadowFocused).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(ui.Hex("#ef4444"))
				ui.Text(c, a.err).FontSize(12.5).TextColor(k.text).MaxWidth(520)
				if iconButton(c, k, "x", tr("Dismiss"), 22, 14).Clicked() {
					a.err = ""
				}
			})
		})
	})
}
