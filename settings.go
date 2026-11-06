package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"

	"zeus/internal/zeus"
)

// settings are the user's choices, kept in settings.json beside the
// server's state.
type settings struct {
	// Appearance is "light", "dark", or "" to follow the system.
	Appearance  string  `json:"appearance,omitempty"`
	FontSize    float32 `json:"fontSize,omitempty"`
	NewTabStart string  `json:"newTabStart,omitempty"`
	ShellPath   string  `json:"shellPath,omitempty"`

	// Language is the language of the interface: "en", "zh", or "" to
	// follow the system's.
	Language string `json:"language,omitempty"`

	// The terminals' text: the family to ask the system for, and how
	// tall a line is against the font's own.
	FontFamily string  `json:"fontFamily,omitempty"`
	LineHeight float32 `json:"lineHeight,omitempty"`

	// ScrollbackMB is how much scrollback a session keeps, in MB, at
	// least 1; below 0 keeps all of it.
	ScrollbackMB int `json:"scrollbackMB,omitempty"`
	// Cursor is "", "bar", or "underline", and CursorBlink is whether it
	// blinks; nil follows the terminal's default, which blinks.
	Cursor       string `json:"cursor,omitempty"`
	CursorBlink  *bool  `json:"cursorBlink,omitempty"`
	CopyOnSelect bool   `json:"copyOnSelect,omitempty"`

	// Shortcuts maps a command's title to the keys the user chose for
	// it, instead of the ones it ships with.
	Shortcuts map[string]shortcutSetting `json:"shortcuts,omitempty"`

	// NewTabDir is the directory new tabs start in when NewTabStart is
	// "choose".
	NewTabDir string `json:"newTabDir,omitempty"`
}

// shortcutSetting is the keys of a command the user changed: the
// modifiers and the key, as ui codes.
type shortcutSetting struct {
	Mods int `json:"mods"`
	Key  int `json:"key"`
}

const defaultFontSize = 16
const defaultFontFamily = "JetBrainsMono Nerd Font Mono"
const defaultFontNLFamily = "JetBrainsMonoNL Nerd Font Mono"

var prefs settings

// terminalFontFamily keeps the selected typeface first and adds the
// embedded Nerd Font and system monospace families for missing glyphs.
func terminalFontFamily(selected string) string {
	if strings.TrimSpace(selected) == "" {
		selected = defaultFontFamily
	}
	var families []string
	seen := map[string]bool{}
	for _, list := range []string{selected, defaultFontFamily, "SF Mono", "Menlo", "monospace"} {
		for _, family := range strings.Split(list, ",") {
			family = strings.TrimSpace(strings.Trim(family, `"'`))
			key := strings.ToLower(family)
			if family == "" || seen[key] {
				continue
			}
			seen[key] = true
			families = append(families, family)
		}
	}
	return strings.Join(families, ", ")
}

func settingsPath() string { return filepath.Join(zeus.Dir(), "settings.json") }

func loadSettings() {
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &prefs)
	}
	if prefs.FontSize < 6 || prefs.FontSize > 40 {
		prefs.FontSize = defaultFontSize
	}
	if prefs.NewTabStart != "home" {
		prefs.NewTabStart = "current"
	}
	if prefs.NewTabStart == "choose" && prefs.NewTabDir == "" {
		prefs.NewTabStart = "current"
	}
	if prefs.FontFamily == "" {
		prefs.FontFamily = defaultFontFamily
	}
	if prefs.LineHeight < 0.8 || prefs.LineHeight > 2 {
		prefs.LineHeight = 1
	}
	if prefs.ScrollbackMB == 0 {
		prefs.ScrollbackMB = 10
	}
	if prefs.ScrollbackMB < 0 {
		prefs.ScrollbackMB = -1
	}
	switch prefs.Cursor {
	case "bar", "underline":
	default:
		prefs.Cursor = ""
	}
	switch prefs.Language {
	case langEn, langZh:
	default:
		prefs.Language = ""
	}
	termFont.Size = prefs.FontSize
	termFont.Family = terminalFontFamily(prefs.FontFamily)
	termFont.LineHeight = prefs.LineHeight
	applyAppearance()
}

func saveSettings() {
	b, _ := json.MarshalIndent(prefs, "", "  ")
	os.MkdirAll(zeus.Dir(), 0o700)
	os.WriteFile(settingsPath(), b, 0o600)
}

func applyAppearance() {
	switch prefs.Appearance {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

func (a *App) setAppearance(v string) {
	prefs.Appearance = v
	applyAppearance()
	saveSettings()
}

func (a *App) setNewTabStart(v string) {
	if v != "home" && v != "choose" {
		v = "current"
	}
	prefs.NewTabStart = v
	saveSettings()
}

// setLanguage chooses the language of the interface, "" to follow the
// system's.
func (a *App) setLanguage(v string) {
	if v != langEn && v != langZh {
		v = ""
	}
	if prefs.Language == v {
		return
	}
	prefs.Language = v
	saveSettings()
	a.rebuildMenu()
	// The views rebuild from the strings of the language on their next
	// frame, and the windows' own titles need setting by hand.
	a.services.Invalidate()
	a.settingsServices.Invalidate()
	if a.settingsWin != nil && !a.settingsWin.IsDestroyed() {
		a.settingsWin.SetTitle(tr("Settings"))
	}
}

// chooseNewTabDir shows the folder picker for new tabs to start in, on
// its own goroutine: the dialog blocks until it is dismissed.
func (a *App) chooseNewTabDir() {
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     tr("Choose a folder for new tabs"),
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		mygo.RunOnMain(func() {
			prefs.NewTabDir, prefs.NewTabStart = paths[0], "choose"
			saveSettings()
		})
	}()
}

// scrollbackBytes is how much scrollback a session keeps, in bytes; -1
// keeps all of it, and 0 asks the terminal for its default.
func scrollbackBytes() int {
	if prefs.ScrollbackMB < 0 {
		return -1
	}
	return prefs.ScrollbackMB << 20
}

// cursorStyleOf is the cursor the terminals start with.
func cursorStyleOf() terminal.CursorStyle {
	switch prefs.Cursor {
	case "bar":
		return terminal.CursorBar
	case "underline":
		return terminal.CursorUnderline
	}
	return terminal.CursorBlock
}

// cursorBlinkOf is whether the cursor blinks unless a program asks
// otherwise.
func cursorBlinkOf() bool {
	if prefs.CursorBlink != nil {
		return *prefs.CursorBlink
	}
	return true
}

// applyFont sets the terminals' font in every pane and the preview.
func (a *App) applyFont() {
	termFont.Family = terminalFontFamily(prefs.FontFamily)
	termFont.LineHeight = prefs.LineHeight
	if a.settingsPreview != nil {
		a.settingsPreview.SetFont(termFont)
	}
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			if p.term != nil {
				p.term.SetFont(termFont)
			}
		}
	}
}

func (a *App) setFontFamily(family string) {
	if family == "" {
		family = defaultFontFamily
	}
	prefs.FontFamily = family
	a.applyFont()
	saveSettings()
}

func (a *App) setLineHeight(height float32) {
	height = min(max(height, 0.8), 2)
	prefs.LineHeight = height
	a.settingsLineHt = float64(height)
	a.applyFont()
	saveSettings()
}

// previewFont shows the current font in the settings' preview alone.
func (a *App) previewFont() {
	if a.settingsPreview != nil {
		a.settingsPreview.SetFont(termFont)
	}
}

// commitFont gives the open terminals the font the sliders chose and
// keeps it: reshaping every terminal is too costly to do as a slider
// moves, so a drag feeds the preview alone and this runs at its end.
func (a *App) commitFont() {
	a.settingsFontDrag, a.settingsLineDrag = false, false
	prefs.FontSize, prefs.LineHeight = termFont.Size, termFont.LineHeight
	a.settingsFontSize, a.settingsLineHt = float64(termFont.Size), float64(termFont.LineHeight)
	a.applyFont()
	saveSettings()
}

// commitFontIfDue lands a slider drag the settings window closed during.
func (a *App) commitFontIfDue() {
	if a.settingsFontDrag || a.settingsLineDrag {
		a.commitFont()
	}
}

// applyPaneBehavior sets the scrollback and the cursor of every pane,
// which the settings change while sessions run.
func (a *App) applyPaneBehavior() {
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			if p.term == nil {
				continue
			}
			p.term.SetScrollback(scrollbackBytes())
			p.term.SetCursor(cursorStyleOf(), cursorBlinkOf())
			p.term.SetCopyOnSelect(prefs.CopyOnSelect)
		}
	}
}

func (a *App) setScrollback(mb int) {
	if mb != -1 {
		mb = min(max(mb, 1), 1024)
	}
	prefs.ScrollbackMB = mb
	a.applyPaneBehavior()
	saveSettings()
}

func (a *App) setCursor(style string) {
	switch style {
	case "bar", "underline":
	default:
		style = ""
	}
	prefs.Cursor = style
	a.applyPaneBehavior()
	saveSettings()
}

func (a *App) setCursorBlink(blink bool) {
	prefs.CursorBlink = &blink
	a.applyPaneBehavior()
	saveSettings()
}

func (a *App) setCopyOnSelect(on bool) {
	prefs.CopyOnSelect = on
	a.applyPaneBehavior()
	saveSettings()
}

// setFontSize changes the size of the terminals' text, in every pane.
func (a *App) setFontSize(size float32) {
	size = min(max(size, 8), 32)
	prefs.FontSize = size
	termFont.Size = size
	a.settingsFontSize = float64(size)
	a.applyFont()
	saveSettings()
}
