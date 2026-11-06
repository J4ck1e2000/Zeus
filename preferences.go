package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

const (
	pageAppearance = "appearance"
	pageTerminal   = "terminal"
	pageNewTabs    = "new-tabs"
	pageShortcuts  = "shortcuts"
)

type settingsPreviewConn struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (c *settingsPreviewConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *settingsPreviewConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *settingsPreviewConn) Close() error {
	_ = c.r.Close()
	return c.w.Close()
}

func newSettingsPreview() (*terminal.Terminal, error) {
	r, w := io.Pipe()
	conn := &settingsPreviewConn{r: r, w: w}
	t, err := terminal.New(terminal.Options{
		Conn:      conn,
		Font:      termFont,
		Theme:     lightTerm,
		DarkTheme: darkTerm,
		NoBlink:   true,
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	t.Feed([]byte(
		"\x1b[48;2;163;174;210m\x1b[38;2;9;12;12m Zeus " +
			"\x1b[48;2;118;159;240m\x1b[38;2;163;174;210m" +
			"\x1b[38;2;227;229;229m ~/Projects " +
			"\x1b[48;2;57;66;96m\x1b[38;2;118;159;240m" +
			"\x1b[48;2;29;34;48m\x1b[38;2;160;169;203m  10:42 " +
			"\x1b[0m\r\n\x1b[32m❯ \x1b[0m",
	))
	return t, nil
}

func (a *App) openSettings() {
	if a.win == nil {
		return
	}
	if a.settingsWin != nil && !a.settingsWin.IsDestroyed() {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	preview, err := newSettingsPreview()
	if err != nil {
		log.Printf("settings preview: %v", err)
	}
	a.settingsPreview = preview
	a.settingsFontSize = float64(termFont.Size)
	a.settingsLineHt = float64(termFont.LineHeight)
	if a.settingsPage == "" {
		a.settingsPage = pageAppearance
	}
	opts := mygo.WindowOptions{
		Title:                tr("Settings"),
		Width:                780,
		Height:               560,
		MinWidth:             640,
		MinHeight:            460,
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 16, Y: 15},
		TitleBarHeight:       titleH,
		BackgroundColor:      "light-dark(#efe1e6, #231e27)",
		Content:              ui.View(a.settingsView),
	}
	if runtime.GOOS != "darwin" {
		// macOS makes the window a child of its parent, and a child window
		// stutters while it drags; Zeus closes the settings itself with the
		// main window, so the tie is only wanted where owned windows are
		// the norm.
		opts.Parent = a.win
	}
	if ownControls {
		opts.TitleBarStyle, opts.Frameless = mygo.TitleBarDefault, true
	}
	win := mygo.NewWindow(opts)
	a.settingsWin = win
	win.OnClosed(func() {
		a.commitFontIfDue()
		if a.settingsPreview != nil {
			_ = a.settingsPreview.Close()
			a.settingsPreview = nil
		}
		a.settingsWin = nil
	})
	win.Show()
}

func (a *App) settingsView(c *ui.Context) {
	k := colorsOf(c)
	setInkAccent(c, k)
	a.settingsServices = c.Services()
	c.Root().Background(k.bgBottom)
	ui.Column(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { paintBackground(p, r, k) }).Children(func() {
		a.settingsTitleBar(c, k)
		ui.Row(c).Grow(1).MinHeight(0).Padding(0, 16, 16, 16).Gap(0).Children(func() {
			a.settingsSidebar(c, k).Width(180).MinWidth(180).Grow(0)
			ui.Column(c).Grow(1).Fill().MinWidth(0).MinHeight(0).Padding(28, 30, 24, 30).Gap(14).Children(func() {
				a.settingsPageView(c, k)
			})
		})
	})
}

// setInkAccent recolors the window's controls (sliders, switches, focus
// rings) from the theme's blue to the app's ink, which the warm gradient
// asks for.
func setInkAccent(c *ui.Context, k *colors) {
	t := *c.Theme()
	t.Accent, t.AccentHover, t.AccentPressed = k.text, k.text, k.text
	if c.Theme().Dark {
		t.AccentText = ui.Hex("#1e1e22")
	} else {
		t.AccentText = ui.Hex("#fbfbfb")
	}
	c.SetTheme(&t)
}

func (a *App) settingsTitleBar(c *ui.Context, k *colors) {
	bar := c.TitleBar()
	left := bar.Left
	if left == 0 {
		left = 12
	} else {
		left += 14
	}
	ui.Row(c).Height(titleH).Padding(0, max(bar.Right, 10), 0, left).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, tr("Settings")).FontSize(14).FontWeight(600).TextColor(k.text)
		ui.Spacer(c).MinWidth(40)
		a.windowControlsFor(c, k, a.settingsWin)
	})
}

func (a *App) settingsSidebar(c *ui.Context, k *colors) ui.Element {
	return ui.Column(c).Fill().Padding(18, 10).Gap(4).Children(func() {
		ui.Text(c, "Zeus").FontSize(15).FontWeight(600).TextColor(k.text).Padding(4, 9, 10, 9)
		a.settingsNavItem(c, k, pageAppearance, tr("Appearance"), "settings-2")
		a.settingsNavItem(c, k, pageTerminal, tr("Terminal"), "terminal")
		a.settingsNavItem(c, k, pageNewTabs, tr("New tabs"), "folder-open")
		a.settingsNavItem(c, k, pageShortcuts, tr("Shortcuts"), "keyboard")
		ui.Spacer(c)
		ui.Text(c, tr("Changes save automatically")).FontSize(11.5).TextColor(k.textFaint).Padding(8, 9, 0, 9)
	})
}

func (a *App) settingsNavItem(c *ui.Context, k *colors, page, label, glyph string) {
	e := ui.Row(c).Height(36).Padding(0, 9).Gap(9).AlignItems(ui.Center).Radius(10).
		Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(label)
	if a.settingsPage == page {
		e.Background(k.raise)
	} else if e.Hovered() {
		e.Background(k.hover)
	}
	if e.Clicked() {
		a.settingsPage = page
	}
	e.Children(func() {
		col := k.textMuted
		if a.settingsPage == page {
			col = k.text
		}
		ui.Icon(c, icon(glyph)).Size(15, 15).TextColor(col)
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(col)
	})
}

func (a *App) settingsPageView(c *ui.Context, k *colors) {
	title, description := tr("Appearance"), tr("Choose how Zeus looks and how terminal text is sized.")
	switch a.settingsPage {
	case pageNewTabs:
		title, description = tr("New tabs"), tr("Choose where a new tab starts. Split panes keep the focused pane's directory.")
	case pageTerminal:
		title, description = tr("Terminal"), tr("Choose the cursor, the scrollback, and how selection copies.")
	case pageShortcuts:
		title, description = tr("Shortcuts"), tr("Keyboard commands available in Zeus.")
	}
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(23).FontWeight(600).TextColor(k.text)
		ui.Text(c, description).FontSize(13).TextColor(k.textMuted)
	})
	switch a.settingsPage {
	case pageTerminal:
		a.settingsScroll(c, k, a.settingsTerminal)
	case pageNewTabs:
		a.settingsScroll(c, k, a.settingsNewTabs)
	case pageShortcuts:
		a.settingsShortcuts(c, k)
	default:
		a.settingsScroll(c, k, a.settingsAppearance)
	}
}

// settingsScroll runs a settings page in a scroll view, so longer pages
// stay whole in a short window.
func (a *App) settingsScroll(c *ui.Context, k *colors, page func(*ui.Context, *colors)) {
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 0, 4, 0).Children(func() {
		ui.Column(c).Gap(14).Children(func() { page(c, k) })
	})
}

// settingsCard groups rows on a raised card, as the panes sit on the
// window's gradient; it takes the place of the row separators.
func settingsCard(c *ui.Context, k *colors, build func()) {
	ui.Column(c).Radius(cardR).Material(paneGlass(k, false)).Border(1, k.cardBorder).
		Shadow(0, 1, 2, 0, k.shadow).Shadow(0, 6, 22, -2, k.shadow).
		Padding(14, 16).Gap(12).Children(build)
}

func (a *App) settingsAppearance(c *ui.Context, k *colors) {
	ui.Column(c).Gap(14).Children(func() {
		settingsCard(c, k, func() {
			settingsRow(c, k, tr("Appearance"), tr("Follow the system appearance or choose light or dark."), func() {
				selected := 0
				if prefs.Appearance == "light" {
					selected = 1
				} else if prefs.Appearance == "dark" {
					selected = 2
				}
				if settingsSegmented(c, k, &selected, tr("Appearance"), tr("System"), tr("Light"), tr("Dark")).Width(260).Changed() {
					choices := [...]string{"", "light", "dark"}
					a.setAppearance(choices[selected])
				}
			})
			settingsRow(c, k, tr("Language"), tr("Choose the language of the interface."), func() {
				selected := 0
				switch prefs.Language {
				case langEn:
					selected = 1
				case langZh:
					selected = 2
				}
				if settingsSegmented(c, k, &selected, tr("Language"), tr("System"), "English", "简体中文").Width(260).Changed() {
					choices := [...]string{"", langEn, langZh}
					a.setLanguage(choices[selected])
				}
			})
		})

		settingsCard(c, k, func() {
			settingsRow(c, k, tr("Terminal text size"), tr("Applies to every open pane."), func() {
				ui.Row(c).Width(292).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
					slider := ui.Slider(c, &a.settingsFontSize, 8, 32).Grow(1).Label(tr("Terminal text size"))
					a.sliderFontSize(slider)
					ui.Text(c, fmt.Sprintf("%.0f", a.settingsFontSize)).Width(28).FontSize(13).TextColor(k.textMuted)
					settingsButton(c, k, tr("Reset"), func() { a.setFontSize(defaultFontSize) })
				})
			})
			settingsRow(c, k, tr("Font"), tr("The family of the terminals' text, from the system's fonts or the fonts Zeus ships."), func() {
				family := prefs.FontFamily
				if settingsSelect(c, k, &family, tr("Font"), fontFamilies()).Changed() {
					a.setFontFamily(family)
				}
			})
			settingsRow(c, k, tr("Line height"), tr("How tall a line of text is against the font's own."), func() {
				ui.Row(c).Width(292).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
					slider := ui.Slider(c, &a.settingsLineHt, 0.8, 2).Grow(1).Label(tr("Line height"))
					a.sliderLineHeight(slider)
					ui.Text(c, fmt.Sprintf("%.2f", a.settingsLineHt)).Width(34).FontSize(13).TextColor(k.textMuted)
					settingsButton(c, k, tr("Reset"), func() { a.setLineHeight(1) })
				})
			})
		})
	})
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, tr("Preview")).FontSize(13).FontWeight(500).TextColor(k.text)
		if a.settingsPreview != nil {
			ui.Column(c).Height(112).MinWidth(0).Radius(cardR).Border(1, k.cardBorder).Clip().Children(func() {
				terminal.View(c, a.settingsPreview).Fill()
			})
		}
	})
}

// sliderFontSize takes the font-size slider's changes. While it drags,
// only the preview takes the size; the open terminals and the saved
// settings do at the drag's end, or at once when the keys move it.
func (a *App) sliderFontSize(slider ui.Element) {
	if slider.Changed() {
		termFont.Size = float32(math.Round(a.settingsFontSize))
		a.previewFont()
		a.settingsFontDrag = slider.Pressed()
		if !a.settingsFontDrag {
			a.commitFont()
		}
		return
	}
	if a.settingsFontDrag && !slider.Pressed() {
		a.commitFont()
	}
}

// sliderLineHeight takes the line-height slider's changes, as the
// font-size slider takes its.
func (a *App) sliderLineHeight(slider ui.Element) {
	if slider.Changed() {
		termFont.LineHeight = float32(math.Round(a.settingsLineHt*20) / 20)
		a.previewFont()
		a.settingsLineDrag = slider.Pressed()
		if !a.settingsLineDrag {
			a.commitFont()
		}
		return
	}
	if a.settingsLineDrag && !slider.Pressed() {
		a.commitFont()
	}
}

// fontFamilies are the families the font picker offers: those Zeus
// ships, the system's own, and common monospaced fonts, with whatever
// the user chose first.
func fontFamilies() []string {
	list := []string{defaultFontFamily, defaultFontNLFamily}
	switch runtime.GOOS {
	case "darwin":
		list = append(list, "SF Mono", "Menlo", "Monaco", "Courier New")
	case "windows":
		list = append(list, "Cascadia Mono", "Consolas", "Courier New")
	default:
		list = append(list, "Monospace", "DejaVu Sans Mono", "Courier New")
	}
	list = append(list, "Fira Code", "JetBrains Mono", "Hack", "Source Code Pro")
	if prefs.FontFamily != "" && !slices.Contains(list, prefs.FontFamily) {
		list = append([]string{prefs.FontFamily}, list...)
	}
	return slices.Compact(slices.Sorted(slices.Values(list)))
}

// settingsTerminal is the page of the terminals' behavior.
func (a *App) settingsTerminal(c *ui.Context, k *colors) {
	ui.Column(c).Gap(14).Children(func() {
		settingsCard(c, k, func() {
			settingsRow(c, k, tr("Cursor"), tr("The shape of the cursor, which programs may change."), func() {
				selected := 0
				switch prefs.Cursor {
				case "bar":
					selected = 1
				case "underline":
					selected = 2
				}
				if settingsSegmented(c, k, &selected, tr("Cursor"), tr("Block"), tr("Bar"), tr("Underline")).Width(280).Changed() {
					choices := [...]string{"", "bar", "underline"}
					a.setCursor(choices[selected])
				}
			})
			settingsRow(c, k, tr("Blinking"), tr("Whether the cursor blinks, unless a program asks otherwise."), func() {
				on := cursorBlinkOf()
				if ui.Switch(c, &on).Changed() {
					a.setCursorBlink(on)
				}
			})
		})

		settingsCard(c, k, func() {
			settingsRow(c, k, tr("Scrollback"), tr("How much output a session keeps above the screen."), func() {
				choices := [...]int{2, 10, 50, -1}
				selected := 3
				for i, v := range choices {
					if prefs.ScrollbackMB == v {
						selected = i
					}
				}
				if settingsSegmented(c, k, &selected, tr("Scrollback"), "2 MB", "10 MB", "50 MB", tr("Unlimited")).Width(340).Changed() {
					a.setScrollback(choices[selected])
				}
			})
			settingsRow(c, k, tr("Copy on select"), tr("Selected text goes to the clipboard as soon as it is made."), func() {
				on := prefs.CopyOnSelect
				if ui.Switch(c, &on).Changed() {
					a.setCopyOnSelect(on)
				}
			})
		})
	})
}

func (a *App) settingsNewTabs(c *ui.Context, k *colors) {
	ui.Column(c).Gap(14).Children(func() {
		settingsCard(c, k, func() {
			settingsRow(c, k, tr("Start in"), tr("Follow the focused pane, open at your home directory, or pick a folder."), func() {
				selected := 0
				switch prefs.NewTabStart {
				case "home":
					selected = 1
				case "choose":
					selected = 2
				}
				if settingsSegmented(c, k, &selected, tr("Start in"), tr("This pane"), tr("Home"), tr("Choose…")).Width(340).Changed() {
					choices := [...]string{"current", "home", "choose"}
					a.setNewTabStart(choices[selected])
					if choices[selected] == "choose" && prefs.NewTabDir == "" {
						a.chooseNewTabDir()
					}
				}
			})
			if prefs.NewTabStart == "choose" {
				settingsRow(c, k, tr("Folder"), tr("New tabs open at this folder."), func() {
					ui.Text(c, shortDir(prefs.NewTabDir)).FontSize(12.5).TextColor(k.textMuted).
						MaxWidth(150).SingleLine().Ellipsis("…")
					settingsButtonWide(c, k, tr("Browse…"), 90, func() { a.chooseNewTabDir() })
				})
			}
		})
		settingsCard(c, k, func() {
			settingsRow(c, k, tr("Shell executable"), tr("Leave empty to use the system login shell. Applies to new sessions."), func() {
				input := ui.TextInputBase(c, &prefs.ShellPath).Width(230).Padding(7, 10).
					Radius(8).Border(1, k.trackBorder).Background(k.track).Placeholder(tr("System default")).Label(tr("Shell executable"))
				if input.Changed() {
					saveSettings()
				}
			})
		})
	})
}

func (a *App) settingsShortcuts(c *ui.Context, k *colors) {
	if a.recording != nil {
		ui.Column(c).Gap(4).Padding(0, 0, 12, 0).Children(func() {
			ui.Text(c, fmt.Sprintf(tr("Press the keys for “%s”, or Esc to leave it as it is."), a.recording.label())).FontSize(12.5).TextColor(k.text)
			if a.recordingErr != "" {
				ui.Text(c, a.recordingErr).FontSize(11.5).TextColor(ui.Hex("#ef4444"))
			}
		})
		a.captureShortcut(c)
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(2, 0).Children(func() {
		settingsCard(c, k, func() {
			for _, cmd := range settingsShortcutCommands() {
				if cmd.keysOf() == "" {
					continue
				}
				a.settingsShortcutCommand(c, k, cmd)
			}
			settingsKey := "Ctrl+,"
			if runtime.GOOS == "darwin" {
				settingsKey = "⌘,"
			}
			settingsShortcutRow(c, k, tr("Settings…"), settingsKey)
		})
	})
}

// settingsShortcutCommand is a row of the shortcuts' page: the command,
// its keys, and Change and Reset for them.
func (a *App) settingsShortcutCommand(c *ui.Context, k *colors, cmd *command) {
	ui.Row(c).Height(38).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, cmd.label()).Grow(1).FontSize(13).TextColor(k.text)
		if a.recording == cmd {
			ui.Text(c, tr("Press keys…")).FontSize(12).TextColor(k.textFaint).Padding(4, 8).Radius(6).Background(k.hover)
			return
		}
		settingsKeys(c, k, cmd.keysOf())
		settingsButton(c, k, tr("Change"), func() { a.recording, a.recordingErr = cmd, "" })
		if _, ok := cmd.overridden(); ok {
			settingsButton(c, k, tr("Reset"), func() {
				delete(prefs.Shortcuts, cmd.Title)
				saveSettings()
				a.rebuildMenu()
			})
		}
	})
}

// captureShortcut takes the keys the user presses in the settings as
// the recording command's shortcut, and refuses those another command
// has or the terminal's own.
func (a *App) captureShortcut(c *ui.Context) {
	cmd := a.recording
	if c.Shortcut(0, ui.KeyEscape) {
		a.recording = nil
		return
	}
	for _, mods := range captureMods() {
		for _, key := range captureKeys {
			if !c.Shortcut(mods, key) {
				continue
			}
			a.applyShortcut(cmd, mods, key)
			return
		}
	}
}

// applyShortcut gives the command the keys, refusing those it already
// has, those another command has, and the terminal's own.
func (a *App) applyShortcut(cmd *command, mods ui.Modifiers, key ui.Key) {
	if cmd.keysOf() == keysGlyph(mods, key) {
		a.recording = nil // the keys stay as they are
		return
	}
	if key >= ui.KeyA && key <= ui.KeyZ || key >= ui.Key0 && key <= ui.Key9 {
		if mods == ui.Ctrl || runtime.GOOS != "darwin" && mods == ui.Ctrl|ui.Alt {
			a.recordingErr = tr("That combination is the terminal's")
			return
		}
	}
	s := shortcut{mods, key}
	for _, other := range allCommands {
		if other == cmd {
			continue
		}
		if other.otherOf() == s || other.accelOf() == menuAccel(mods, key) && menuAccel(mods, key) != "" {
			a.recordingErr = fmt.Sprintf(tr("Already used by %s"), other.label())
			return
		}
	}
	if prefs.Shortcuts == nil {
		prefs.Shortcuts = map[string]shortcutSetting{}
	}
	prefs.Shortcuts[cmd.Title] = shortcutSetting{Mods: int(mods), Key: int(key)}
	saveSettings()
	a.recording = nil
	a.rebuildMenu()
}

func settingsShortcutCommands() []*command {
	return []*command{
		&cmdNewTab, &cmdSplitRight, &cmdSplitDown, &cmdZoom, &cmdEqualize, &cmdClosePane, &cmdCloseTab,
		&cmdNextTab, &cmdPrevTab, &cmdRenameTab, &cmdPalette, &cmdPalette2,
		&cmdFind,
		&cmdFocusLeft, &cmdFocusRight, &cmdFocusUp, &cmdFocusDown,
		&cmdGrowLeft, &cmdGrowRight, &cmdGrowUp, &cmdGrowDown,
		&cmdClear, &cmdBigger, &cmdSmaller, &cmdActualSize, &cmdEndAll,
	}
}

func settingsShortcutRow(c *ui.Context, k *colors, title, keys string) {
	ui.Row(c).Height(38).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Grow(1).FontSize(13).TextColor(k.text)
		settingsKeys(c, k, keys)
	})
}

// settingsKeys is a shortcut's keys as a keycap.
func settingsKeys(c *ui.Context, k *colors, keys string) {
	ui.Text(c, keys).FontSize(12).TextColor(k.textMuted).Padding(4, 8).Radius(6).
		Background(k.hover).Border(0.5, k.trackBorder)
}

func settingsSegmented(c *ui.Context, k *colors, selected *int, label string, labels ...string) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.AlignItems(ui.Stretch).Gap(2).Padding(2).Label(label).
		Background(k.track).Border(0.5, k.trackBorder).Radius(16)
	track.Children(func() {
		for i, text := range labels {
			segment := parts.Segment(i).Grow(1).Basis(0).MinWidth(0).Height(32).
				Padding(0, 10).Radius(14).Cursor(ui.CursorPointer)
			col := k.textMuted
			if *selected == i {
				segment.Height(28).Background(k.tabActive).
					Shadow(0, 1, 2, 0, k.shadow).Shadow(0, 2, 8, 0, k.shadow)
				col = k.text
			} else if segment.Hovered() {
				segment.Background(k.hover)
			}
			segment.Transition(ui.ElementTransition{Colors: true, Position: true, Duration: 160 * time.Millisecond})
			segment.Children(func() {
				ui.Text(c, text).FontSize(13).FontWeight(500).TextColor(col).SingleLine()
			})
		}
	})
	return track
}

// settingsSelect draws the settings' drop-down in the app's own look: a
// trigger matching the rows' other controls, opening a panel of options.
func settingsSelect(c *ui.Context, k *colors, selected *string, label string, options []string) ui.Element {
	sel := ui.SelectBase(c, selected)
	trigger := sel.Trigger.Label(label).Width(260).Height(32).Padding(0, 11).Gap(8).
		Background(k.track).Border(0.5, k.trackBorder).Radius(9).Cursor(ui.CursorPointer)
	open := sel.Open()
	if trigger.Pressed() {
		trigger.Background(k.facePress)
	} else if open || trigger.Hovered() {
		trigger.Background(k.raise)
	}
	trigger.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	trigger.Children(func() {
		ui.Text(c, *selected).FontSize(13).FontWeight(500).TextColor(k.text).Grow(1).SingleLine()
		chevron := ui.Icon(c, icon("chevron-down")).Size(13, 13).TextColor(k.textMuted).Shrink(0)
		if open {
			chevron.Rotate(180)
		}
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(5).Radius(14).Material(panelGlass(k)).Border(1, k.panelBorder).
			Shadow(0, 1, 2, 0, k.shadowFocused).Shadow(0, 14, 34, -8, k.shadowFocused)
		for _, option := range options {
			item := sel.Item(option).Height(30).Padding(0, 10).Gap(8).
				AlignItems(ui.Center).Radius(8).Cursor(ui.CursorPointer)
			if item.Highlighted() {
				item.Background(k.panelSel)
			}
			item.Transition(ui.ElementTransition{Colors: true, Duration: 100 * time.Millisecond})
			item.Children(func() {
				ui.Text(c, option).FontSize(13).TextColor(k.text).Grow(1).SingleLine()
				if option == *selected {
					ui.Icon(c, icon("check")).Size(12, 12).TextColor(k.textMuted).Shrink(0)
				}
			})
		}
	})
	return trigger
}

func settingsRow(c *ui.Context, k *colors, title, description string, control func()) {
	ui.Row(c).Gap(18).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(500).TextColor(k.text)
			ui.Text(c, description).FontSize(12).TextColor(k.textMuted).Wrap()
		})
		control()
	})
}

func settingsButton(c *ui.Context, k *colors, label string, run func()) {
	settingsButtonWide(c, k, label, 64, run)
}

func settingsButtonWide(c *ui.Context, k *colors, label string, width float32, run func()) {
	b := ui.Box(c).Size(width, 30).Center().Radius(8).Cursor(ui.CursorPointer).
		Role(ui.RoleButton).Label(label)
	if b.Pressed() {
		b.Background(k.facePress)
	} else if b.Hovered() {
		b.Background(k.hover)
	} else {
		b.Background(k.hover)
	}
	b.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
	if b.Clicked() {
		run()
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(k.text) })
}
