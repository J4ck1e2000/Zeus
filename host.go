package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// hostChip shows the machine the sessions run on; a click shows more of
// it, and of the session server.
func (a *App) hostChip(c *ui.Context, k *colors) {
	h := a.hello.Host
	name, model := h.Name, h.Model
	if name == "" {
		name, _ = os.Hostname()
		name = strings.TrimSuffix(name, ".local")
	}
	glyph := "mac-studio"
	switch {
	case strings.Contains(model, "MacBook"), h.Portable:
		glyph = "laptop"
	case strings.Contains(model, "iMac"), runtime.GOOS == "windows":
		glyph = "monitor"
	case model == "Linux":
		glyph = "server"
	}
	chip := ui.Row(c).Height(28).Gap(9).Padding(3, 10, 3, 6).Radius(14).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Material(chromeGlass(k)).Border(0.5, k.trackBorder).
		Role(ui.RoleButton).Label(fmt.Sprintf(tr("Host %s"), name)).MaxWidth(230).Shrink(0)
	if chip.Hovered() || a.hostOpen {
		chip.Background(k.hover)
	}
	if chip.Clicked() {
		a.hostOpen = !a.hostOpen
		a.hostHiding = !a.hostOpen
		if a.hostOpen {
			a.paletteOpen = false // one overlay at a time
		}
	}
	chip.Children(func() {
		ui.Icon(c, icon(glyph)).Size(17, 17).TextColor(k.textMuted)
		ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(k.text.Alpha(0.85)).SingleLine().Ellipsis("…")
	})
	// The panel fades in and out by hand: the layer it is born in exists
	// only while it shows, so the framework's Enter and Exit, which look
	// for a parent the frame before showed, have nothing to hold on to. A
	// close no one timed fades from where it was.
	if !a.hostOpen && !a.hostHiding && a.hostWasShown {
		a.hostHiding = true
	}
	shown := a.hostOpen || a.hostHiding
	fresh := shown && !a.hostWasShown
	a.hostWasShown = shown
	var v float32 // how shown the panel is
	ui.PopoverBase(c, chip, &shown, func(panel ui.Element) {
		target := float32(0)
		if a.hostOpen {
			target = 1
		}
		if fresh {
			panel.Animate("host show", 0, dialogFade) // start from nothing
		}
		v = panel.Animate("host show", target, dialogFade)
		// The panel rises the last little way as it settles under the chip.
		panel.Opacity(v).Margin(8+(1-v)*6, 0, 0, 0).Width(300).Padding(14).Radius(18).
			Material(panelGlass(k)).Border(1, k.panelBorder).
			Shadow(0, 12, 32, 0, k.shadowFocused).Gap(12)
		if a.hostOpen {
			// The panel takes the keyboard focus while it shows: the
			// focused terminal would keep the keys, and Escape would
			// never reach it.
			panel.AutoFocus()
		}
		a.hostPanel(c, k, name, glyph)
	})
	if a.hostOpen && !shown { // Escape, or a click outside it, closed it
		a.hostOpen = false
	}
	if !a.hostOpen && a.hostHiding && v == 0 {
		// The fade is over; the keys go back to the pane, unless another
		// overlay has taken them while this one faded. WasShown goes too,
		// or the catch-all close above would rebuild this invisible panel
		// every frame, and it would take the presses of the title bar.
		a.hostHiding = false
		a.hostWasShown = false
		c.After(time.Millisecond) // a frame without the panel, so it stops taking presses
		if !a.paletteOpen && !a.paletteHiding {
			if t := a.tab(); t != nil && a.focusReq == nil {
				a.focusReq = t.Focus
			}
		}
	}
}

func (a *App) hostPanel(c *ui.Context, k *colors, name, glyph string) {
	h := a.hello.Host
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(38, 38).Radius(10).Background(k.hover).Center().Children(func() {
			ui.Icon(c, icon(glyph)).Size(24, 24).TextColor(k.text)
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, name).FontSize(14).FontWeight(700).TextColor(k.text).SingleLine().Ellipsis("…")
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Background(k.busy)
				ui.Text(c, thisMachine()).FontSize(12).TextColor(k.textFaint)
			})
		})
	})
	ui.Divider(c)
	row := func(label, value string) {
		if value == "" {
			return
		}
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, label).FontSize(12.5).TextColor(k.textFaint).Width(78)
			ui.Text(c, value).FontSize(12.5).TextColor(k.text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		})
	}
	ui.Column(c).Gap(7).Children(func() {
		row(tr("Model"), h.Model)
		row(tr("Chip"), h.Chip)
		if h.Memory > 0 {
			row(tr("Memory"), fmt.Sprintf("%d GB", h.Memory>>30))
		}
		row(tr("System"), h.OS)
		row(tr("User"), h.User)
	})
	ui.Divider(c)
	sessions, running := 0, 0
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			sessions++
			if !p.info.Idle && p.info.Program != "" {
				running++
			}
		}
	}
	ui.Column(c).Gap(7).Children(func() {
		row(tr("Server"), fmt.Sprintf(tr("pid %d · up %s"), a.hello.PID, roundDur(time.Since(a.hello.Started))))
		row(tr("Sessions"), fmt.Sprintf(tr("%d open, %d running a program"), sessions, running))
	})
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		settingsButton(c, k, tr("Quit"), func() { a.quitAndEnd() })
		ui.Text(c, tr("and end all the sessions")).FontSize(11.5).TextColor(k.textFaint)
	})
	ui.Text(c, tr("Sessions live in the server: quit Zeus and they keep running, and come back as they were when it opens again.")).
		FontSize(11.5).TextColor(k.textFaint).LineHeight(1.35)
}

func roundDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// thisMachine names the machine the app runs on, as its system does.
func thisMachine() string {
	switch runtime.GOOS {
	case "darwin":
		return tr("This Mac")
	case "windows":
		return tr("This PC")
	}
	return tr("This computer")
}
