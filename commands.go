package main

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// command is an action of the menus and the command palette.
type command struct {
	Title string
	// Accel is the menu item's shortcut, and Keys how the palette shows it.
	Accel, Keys string
	// Other is the shortcut elsewhere than on macOS, which gives Accel and
	// Keys there: Control and a letter are the terminal's, so Shift comes
	// with it, as when copying and pasting; Alt and Control-Alt make
	// characters (AltGr), and Alt and a letter open the menus on Windows.
	Other shortcut
	Run   func(a *App)
	// Hidden keeps it out of the palette.
	Hidden bool
}

// label is the command's title in the language the interface shows in.
// Title stays English, as the key the shortcuts are kept by.
func (c *command) label() string { return tr(c.Title) }

var (
	cmdNewTab      = command{Title: "New Tab", Accel: "CmdOrCtrl+T", Keys: "⌘T", Other: ctrlShift(ui.KeyT), Run: func(a *App) { a.newTab(a.newTabDir()) }}
	cmdSplitRight  = command{Title: "Split Right", Accel: "CmdOrCtrl+D", Keys: "⌘D", Other: ctrlShift(ui.KeyD), Run: func(a *App) { a.split(false) }}
	cmdSplitDown   = command{Title: "Split Down", Accel: "CmdOrCtrl+Shift+D", Keys: "⇧⌘D", Other: ctrlShift(ui.KeyE), Run: func(a *App) { a.split(true) }}
	cmdClosePane   = command{Title: "Close Pane", Accel: "CmdOrCtrl+W", Keys: "⌘W", Other: ctrlShift(ui.KeyW), Run: func(a *App) { a.closeFocused() }}
	cmdCloseTab    = command{Title: "Close Tab", Accel: "CmdOrCtrl+Shift+W", Keys: "⇧⌘W", Run: func(a *App) { a.closeActiveTab() }}
	cmdZoom        = command{Title: "Zoom Pane", Accel: "CmdOrCtrl+Shift+Enter", Keys: "⇧⌘↩", Other: ctrlShift(ui.KeyEnter), Run: func(a *App) { a.toggleZoom() }}
	cmdEqualize    = command{Title: "Equalize Panes", Accel: "CmdOrCtrl+Ctrl+=", Keys: "⌃⌘=", Run: func(a *App) { a.equalize() }}
	cmdNextTab     = command{Title: "Next Tab", Accel: "CmdOrCtrl+Shift+]", Keys: "⇧⌘]", Other: shortcut{ui.Ctrl, ui.KeyPageDown}, Run: func(a *App) { a.cycleTab(1) }}
	cmdPrevTab     = command{Title: "Previous Tab", Accel: "CmdOrCtrl+Shift+[", Keys: "⇧⌘[", Other: shortcut{ui.Ctrl, ui.KeyPageUp}, Run: func(a *App) { a.cycleTab(-1) }}
	cmdRenameTab   = command{Title: "Rename Tab…", Accel: "CmdOrCtrl+Shift+R", Keys: "⇧⌘R", Other: ctrlShift(ui.KeyR), Run: func(a *App) { a.startRename() }}
	cmdPalette     = command{Title: "Command Palette…", Accel: "CmdOrCtrl+Shift+P", Keys: "⇧⌘P", Other: ctrlShift(ui.KeyP), Run: func(a *App) { a.openPalette() }, Hidden: true}
	cmdPalette2    = command{Title: "Go to Pane…", Accel: "CmdOrCtrl+P", Keys: "⌘P", Run: func(a *App) { a.openPalette() }, Hidden: true}
	cmdFileTree    = command{Title: "Toggle File Tree", Run: func(a *App) { a.toggleFileTree() }}
	cmdFind        = command{Title: "Find…", Accel: "CmdOrCtrl+F", Keys: "⌘F", Other: ctrlShift(ui.KeyF), Run: func(a *App) { a.openFind() }}
	cmdSettings    = command{Title: "Settings…", Accel: "CmdOrCtrl+,", Keys: "⌘,", Other: shortcut{ui.Ctrl, ui.KeyComma}, Run: func(a *App) { a.openSettings() }}
	cmdFocusLeft   = command{Title: "Focus Pane Left", Accel: "CmdOrCtrl+Alt+Left", Keys: "⌥⌘←", Other: shortcut{ui.Alt, ui.KeyLeft}, Run: func(a *App) { a.moveFocus(-1, 0) }}
	cmdFocusRight  = command{Title: "Focus Pane Right", Accel: "CmdOrCtrl+Alt+Right", Keys: "⌥⌘→", Other: shortcut{ui.Alt, ui.KeyRight}, Run: func(a *App) { a.moveFocus(1, 0) }}
	cmdFocusUp     = command{Title: "Focus Pane Above", Accel: "CmdOrCtrl+Alt+Up", Keys: "⌥⌘↑", Other: shortcut{ui.Alt, ui.KeyUp}, Run: func(a *App) { a.moveFocus(0, -1) }}
	cmdFocusDown   = command{Title: "Focus Pane Below", Accel: "CmdOrCtrl+Alt+Down", Keys: "⌥⌘↓", Other: shortcut{ui.Alt, ui.KeyDown}, Run: func(a *App) { a.moveFocus(0, 1) }}
	cmdGrowLeft    = command{Title: "Move Divider Left", Accel: "CmdOrCtrl+Ctrl+Left", Keys: "⌃⌘←", Other: shortcut{ui.Alt | ui.Shift, ui.KeyLeft}, Run: func(a *App) { a.resizeFocused(-1, 0) }}
	cmdGrowRight   = command{Title: "Move Divider Right", Accel: "CmdOrCtrl+Ctrl+Right", Keys: "⌃⌘→", Other: shortcut{ui.Alt | ui.Shift, ui.KeyRight}, Run: func(a *App) { a.resizeFocused(1, 0) }}
	cmdGrowUp      = command{Title: "Move Divider Up", Accel: "CmdOrCtrl+Ctrl+Up", Keys: "⌃⌘↑", Other: shortcut{ui.Alt | ui.Shift, ui.KeyUp}, Run: func(a *App) { a.resizeFocused(0, -1) }}
	cmdGrowDown    = command{Title: "Move Divider Down", Accel: "CmdOrCtrl+Ctrl+Down", Keys: "⌃⌘↓", Other: shortcut{ui.Alt | ui.Shift, ui.KeyDown}, Run: func(a *App) { a.resizeFocused(0, 1) }}
	cmdClear       = command{Title: "Clear Screen and Scrollback", Accel: "CmdOrCtrl+K", Keys: "⌘K", Other: ctrlShift(ui.KeyK), Run: func(a *App) { a.clearFocused() }, Hidden: true}
	cmdClearScroll = command{Title: "Clear Scrollback", Run: func(a *App) { a.clearFocused() }}
	cmdRestart     = command{Title: "Restart Shell in Pane", Run: func(a *App) { a.restartFocused() }}
	cmdBigger      = command{Title: "Bigger Text", Accel: "CmdOrCtrl+=", Keys: "⌘+", Other: shortcut{ui.Ctrl, ui.KeyEqual}, Run: func(a *App) { a.setFontSize(termFont.Size + 1) }}
	cmdSmaller     = command{Title: "Smaller Text", Accel: "CmdOrCtrl+-", Keys: "⌘−", Other: shortcut{ui.Ctrl, ui.KeyMinus}, Run: func(a *App) { a.setFontSize(termFont.Size - 1) }}
	cmdActualSize  = command{Title: "Actual Size", Accel: "CmdOrCtrl+0", Keys: "⌘0", Other: shortcut{ui.Ctrl, ui.Key0}, Run: func(a *App) { a.setFontSize(defaultFontSize) }}
	cmdLight       = command{Title: "Appearance: Light", Run: func(a *App) { a.setAppearance("light") }}
	cmdDark        = command{Title: "Appearance: Dark", Run: func(a *App) { a.setAppearance("dark") }}
	cmdSystem      = command{Title: "Appearance: System", Run: func(a *App) { a.setAppearance("") }}
	cmdEndAll      = command{Title: "Quit and End All Sessions", Accel: "CmdOrCtrl+Alt+Q", Keys: "⌥⌘Q", Run: func(a *App) { a.quitAndEnd() }}
)

// paletteCommands are those the command palette lists, before the tabs.
var paletteCommands = []*command{
	&cmdNewTab, &cmdSplitRight, &cmdSplitDown, &cmdZoom, &cmdEqualize, &cmdClosePane, &cmdCloseTab,
	&cmdNextTab, &cmdPrevTab, &cmdRenameTab,
	&cmdFileTree, &cmdFind,
	&cmdFocusLeft, &cmdFocusRight, &cmdFocusUp, &cmdFocusDown,
	&cmdGrowLeft, &cmdGrowRight, &cmdGrowUp, &cmdGrowDown,
	&cmdSettings,
	&cmdBigger, &cmdSmaller, &cmdActualSize, &cmdLight, &cmdDark, &cmdSystem,
	&cmdClearScroll, &cmdRestart, &cmdEndAll,
}

// commands are all of them.
var commands = append(slices.Clone(paletteCommands), &cmdPalette, &cmdPalette2, &cmdClear)

func init() {
	if runtime.GOOS == "darwin" {
		return
	}
	for _, cmd := range commands {
		cmd.Accel, cmd.Keys = "", ""
		if cmd.Other != (shortcut{}) {
			cmd.Accel = cmd.Other.String()
			cmd.Keys = cmd.Accel
		}
	}
}

// shortcut is a key with modifiers, elsewhere than on macOS.
type shortcut struct {
	Mods ui.Modifiers
	Key  ui.Key
}

func ctrlShift(k ui.Key) shortcut { return shortcut{ui.Ctrl | ui.Shift, k} }

// showTabKey is the shortcut of the nth tab, from 1 to 9.
func showTabKey(n int) shortcut { return ctrlShift(ui.Key0 + ui.Key(n)) }

// overridden is the keys the user chose for the command, when they did.
func (c *command) overridden() (shortcut, bool) {
	if s, ok := prefs.Shortcuts[c.Title]; ok {
		return shortcut{ui.Modifiers(s.Mods), ui.Key(s.Key)}, true
	}
	return shortcut{}, false
}

// accelOf is the command's shortcut as its menu takes it.
func (c *command) accelOf() string {
	if s, ok := c.overridden(); ok {
		return menuAccel(s.Mods, s.Key)
	}
	return c.Accel
}

// keysOf is how the command's shortcut shows, as in the palette.
func (c *command) keysOf() string {
	if s, ok := c.overridden(); ok {
		return keysGlyph(s.Mods, s.Key)
	}
	return c.Keys
}

// otherOf is the shortcut the command takes elsewhere than on macOS.
func (c *command) otherOf() shortcut {
	if s, ok := c.overridden(); ok {
		return s
	}
	return c.Other
}

// menuAccel writes the modifiers and key as a menu takes them, with
// CmdOrCtrl where the command key goes, as the shipped ones do.
func menuAccel(mods ui.Modifiers, key ui.Key) string {
	name := keyName(key)
	if name == "" {
		return ""
	}
	var parts []string
	if mods&ui.Super != 0 {
		parts = append(parts, "CmdOrCtrl")
	}
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+")
}

// keysGlyph writes the modifiers and key as shortcuts show: ⇧⌘D.
func keysGlyph(mods ui.Modifiers, key ui.Key) string {
	glyph := keyGlyph(key)
	if glyph == "" {
		return ""
	}
	s := ""
	if mods&ui.Shift != 0 {
		s += "⇧"
	}
	if mods&ui.Ctrl != 0 {
		s += "⌃"
	}
	if mods&ui.Alt != 0 {
		s += "⌥"
	}
	if mods&ui.Super != 0 {
		s += "⌘"
	}
	return s + glyph
}

// keyName writes the key as a menu accelerator takes it, or "" when the
// key is not one it can take.
func keyName(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + key - ui.Key0))
	}
	names := map[ui.Key]string{
		ui.KeyEnter: "Enter", ui.KeySpace: "Space", ui.KeyTab: "Tab",
		ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete",
		ui.KeyHome: "Home", ui.KeyEnd: "End",
		ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
		ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
		ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";",
		ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]",
		ui.KeyBackslash: "\\", ui.KeyBackquote: "`", ui.KeyMinus: "-", ui.KeyEqual: "=",
		ui.KeyF1: "F1", ui.KeyF2: "F2", ui.KeyF3: "F3", ui.KeyF4: "F4",
		ui.KeyF5: "F5", ui.KeyF6: "F6", ui.KeyF7: "F7", ui.KeyF8: "F8",
		ui.KeyF9: "F9", ui.KeyF10: "F10", ui.KeyF11: "F11", ui.KeyF12: "F12",
	}
	return names[key]
}

// keyGlyph writes the key as a shortcut shows it.
func keyGlyph(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + key - ui.Key0))
	}
	return map[ui.Key]string{
		ui.KeyEnter: "↩", ui.KeySpace: "Space", ui.KeyTab: "⇥",
		ui.KeyBackspace: "⌫", ui.KeyDelete: "⌦",
		ui.KeyPageUp: "⇞", ui.KeyPageDown: "⇟", ui.KeyHome: "↖", ui.KeyEnd: "↘",
		ui.KeyLeft: "←", ui.KeyRight: "→", ui.KeyUp: "↑", ui.KeyDown: "↓",
		ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";",
		ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]",
		ui.KeyBackslash: "\\", ui.KeyBackquote: "`", ui.KeyMinus: "-", ui.KeyEqual: "=",
		ui.KeyF1: "F1", ui.KeyF2: "F2", ui.KeyF3: "F3", ui.KeyF4: "F4",
		ui.KeyF5: "F5", ui.KeyF6: "F6", ui.KeyF7: "F7", ui.KeyF8: "F8",
		ui.KeyF9: "F9", ui.KeyF10: "F10", ui.KeyF11: "F11", ui.KeyF12: "F12",
	}[key]
}

// captureKeys are the keys a new shortcut may end in, and captureMods
// the modifiers it may hold: at least one of them, for the keys of the
// terminal stay free without any.
var captureKeys = []ui.Key{
	ui.KeyA, ui.KeyB, ui.KeyC, ui.KeyD, ui.KeyE, ui.KeyF, ui.KeyG, ui.KeyH,
	ui.KeyI, ui.KeyJ, ui.KeyK, ui.KeyL, ui.KeyM, ui.KeyN, ui.KeyO, ui.KeyP,
	ui.KeyQ, ui.KeyR, ui.KeyS, ui.KeyT, ui.KeyU, ui.KeyV, ui.KeyW, ui.KeyX,
	ui.KeyY, ui.KeyZ,
	ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7,
	ui.Key8, ui.Key9,
	ui.KeyComma, ui.KeyPeriod, ui.KeySlash, ui.KeySemicolon, ui.KeyQuote,
	ui.KeyBracketLeft, ui.KeyBracketRight, ui.KeyBackslash, ui.KeyBackquote,
	ui.KeyMinus, ui.KeyEqual,
	ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown,
	ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6,
	ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12,
}

// captureMods are the modifier combinations a new shortcut may hold, on
// the command key or the control key: those the menus and the palette
// follow.
func captureMods() []ui.Modifiers {
	base := ui.Super
	if runtime.GOOS != "darwin" {
		base = ui.Ctrl
	}
	return []ui.Modifiers{
		base, base | ui.Shift, base | ui.Alt, base | ui.Shift | ui.Alt,
		ui.Ctrl, ui.Ctrl | ui.Shift, ui.Ctrl | ui.Alt, ui.Ctrl | ui.Shift | ui.Alt,
	}
}

// String writes the shortcut as menus take it, and as Windows shows it:
// "Ctrl+Shift+T".
func (s shortcut) String() string {
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}} {
		if s.Mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	var key string
	switch k := s.Key; {
	case k >= ui.KeyA && k <= ui.KeyZ:
		key = string(rune('A' + k - ui.KeyA))
	case k >= ui.Key0 && k <= ui.Key9:
		key = string(rune('0' + k - ui.Key0))
	default:
		key = map[ui.Key]string{
			ui.KeyEnter: "Enter", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
			ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
			ui.KeyEqual: "=", ui.KeyMinus: "-",
		}[k]
	}
	return strings.Join(append(parts, key), "+")
}

// menu builds the menu bar, whose items run commands in the window.
func (a *App) menu() *mygo.Menu {
	item := func(cmd *command) *mygo.MenuItem {
		return &mygo.MenuItem{Label: cmd.Title, Accelerator: cmd.accelOf(), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.run(cmd)
		}}
	}
	tabItems := []*mygo.MenuItem{item(&cmdNextTab), item(&cmdPrevTab), mygo.Separator()}
	for i := 1; i <= 9; i++ {
		i := i
		accel := fmt.Sprintf("CmdOrCtrl+%d", i)
		if runtime.GOOS != "darwin" {
			accel = showTabKey(i).String()
		}
		tabItems = append(tabItems, &mygo.MenuItem{
			Label: fmt.Sprintf(tr("Show Tab %d"), i), Accelerator: accel,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.do(func() { a.showTab(i) })
			},
		})
	}
	appearance := func(label, v string) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: prefs.Appearance == v, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.setAppearance(v)
		}}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: tr("Shell"), Submenu: []*mygo.MenuItem{
			item(&cmdNewTab),
			mygo.Separator(),
			item(&cmdSplitRight),
			item(&cmdSplitDown),
			mygo.Separator(),
			item(&cmdClosePane),
			item(&cmdCloseTab),
			mygo.Separator(),
			item(&cmdRestart),
			item(&cmdEndAll),
		}},
		{Label: tr("Edit"), Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo},
			{Role: mygo.RoleRedo},
			mygo.Separator(),
			{Role: mygo.RoleCut},
			{Role: mygo.RoleCopy},
			{Role: mygo.RolePaste},
			{Role: mygo.RoleSelectAll},
			mygo.Separator(),
			item(&cmdClear),
			item(&cmdFind),
		}},
		{Label: tr("View"), Submenu: []*mygo.MenuItem{
			item(&cmdSettings),
			mygo.Separator(),
			item(&cmdPalette),
			item(&cmdPalette2),
			mygo.Separator(),
			item(&cmdZoom),
			item(&cmdEqualize),
			mygo.Separator(),
			{Label: tr("Focus"), Submenu: []*mygo.MenuItem{item(&cmdFocusLeft), item(&cmdFocusRight), item(&cmdFocusUp), item(&cmdFocusDown)}},
			{Label: tr("Resize"), Submenu: []*mygo.MenuItem{item(&cmdGrowLeft), item(&cmdGrowRight), item(&cmdGrowUp), item(&cmdGrowDown)}},
			mygo.Separator(),
			item(&cmdBigger),
			item(&cmdSmaller),
			item(&cmdActualSize),
			{Label: tr("Appearance"), Submenu: []*mygo.MenuItem{
				appearance(tr("System"), ""),
				appearance(tr("Light"), "light"),
				appearance(tr("Dark"), "dark"),
			}},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
			{Role: mygo.RoleToggleDevTools, Label: tr("Inspector"), Hidden: !mygo.IsDev()},
		}},
		{Label: tr("Tabs"), Submenu: append(tabItems, mygo.Separator(), item(&cmdRenameTab))},
		{Role: mygo.RoleWindowMenu},
	})
}

// do changes the state on the main thread, from a menu, and draws.
func (a *App) do(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(fn)
}

// buildMenu makes the menu bar, set when the app starts: the shortcuts
// the user changes rebuild the menus through it, which an App method
// could not, being of a cycle with the commands' own initializers.
var buildMenu func() *mygo.Menu

// allCommands is every command, set when the app starts: the shortcuts'
// capture checks them for conflicts, which the commands' own package
// variables could not be of, being of a cycle with the settings' view.
var allCommands []*command

// rebuildMenu applies the shortcuts the user changed to the menus.
func (a *App) rebuildMenu() {
	if buildMenu != nil {
		mygo.App.SetMenu(buildMenu())
	}
}

// run runs a command of the menus, on the main thread. New Tab opens the
// window again when it was closed.
func (a *App) run(cmd *command) {
	if a.win == nil {
		if cmd == &cmdNewTab {
			mygo.RunOnMain(a.open)
		} else if cmd == &cmdSettings {
			mygo.RunOnMain(func() {
				a.open()
				if a.win != nil {
					a.openSettings()
				}
			})
		}
		return
	}
	a.do(func() {
		if a.win != nil {
			cmd.Run(a)
		}
	})
}

func (a *App) closeFocused() {
	if t := a.tab(); t != nil && t.Focus != nil {
		a.closePane(t.Focus)
	}
}

func (a *App) closeActiveTab() {
	if t := a.tab(); t != nil {
		a.closeTab(t)
	}
}

// showTab shows the nth tab, from 1; 9 is the last.
func (a *App) showTab(n int) {
	if n == 9 {
		a.selectTab(len(a.tabs) - 1)
	} else {
		a.selectTab(n - 1)
	}
}

func (a *App) cycleTab(d int) {
	if n := len(a.tabs); n > 1 {
		a.selectTab(((a.active+d)%n + n) % n)
	}
}

func (a *App) startRename() {
	if t := a.tab(); t != nil {
		a.renaming, a.renameText, a.renameFocus = t, t.Name, false
		a.focusReq = nil // the field takes the focus
	}
}

func (a *App) clearFocused() {
	if t := a.tab(); t != nil && t.Focus != nil && t.Focus.term != nil {
		// Clear the scrollback and the screen, and have the shell draw
		// its prompt again, as Command-K does in Terminal.
		t.Focus.term.Feed([]byte("\x1b[H\x1b[2J\x1b[3J"))
		if t.Focus.info.Idle {
			t.Focus.term.Send([]byte{0x0c})
		}
	}
}

// restartFocused ends the focused pane's session and starts a shell in
// its place, in the same directory.
func (a *App) restartFocused() {
	t := a.tab()
	if t == nil || t.Focus == nil {
		return
	}
	old := t.Focus
	cols, rows := 80, 24
	if old.term != nil {
		cols, rows = old.term.Size()
	}
	p := a.newPane(a.currentDir(), cols, rows)
	p.Tab, p.Node = t, old.Node
	old.Node.Pane = p
	old.closed = true
	if old.term != nil {
		old.term.Close()
	}
	go a.client.Kill(old.SID)
	if t.Zoom == old {
		t.Zoom = p
	}
	t.Focus = nil
	t.setFocus(p)
	a.focusReq = p
	a.changed()
}

func (a *App) quitAndEnd() {
	a.quitting = true
	for _, t := range a.tabs {
		for _, p := range t.panes() {
			p.closed = true
		}
	}
	c := a.client
	go func() {
		c.Shutdown()
		mygo.App.Quit()
	}()
}

func (a *App) openPalette() {
	a.paletteOpen, a.paletteQuery, a.paletteSel = true, "", 0
	a.hostOpen = false // one overlay at a time
	a.focusReq = nil   // the palette takes the focus
}

// shortcuts handles the keys the menus do not: Control-Tab between tabs,
// and on Windows, where windows of native UI run no menu shortcuts, those
// of the commands. The window's shortcuts come before the terminal's keys.
func (a *App) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Ctrl, ui.KeyTab) {
		a.cycleTab(1)
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab) {
		a.cycleTab(-1)
	}
	if runtime.GOOS != "windows" {
		return
	}
	for _, cmd := range commands {
		if s := cmd.otherOf(); s != (shortcut{}) && c.Shortcut(s.Mods, s.Key) {
			a.later(c, func() { cmd.Run(a) })
		}
	}
	for i := 1; i <= 9; i++ {
		if k := showTabKey(i); c.Shortcut(k.Mods, k.Key) {
			a.later(c, func() { a.showTab(i) })
		}
	}
}

// paletteItem is a row of the command palette: a command, or a pane to
// go to.
type paletteItem struct {
	title, detail, keys string
	glyph               string
	run                 func()
}

func (a *App) paletteItems() []paletteItem {
	var founds []found
	q := strings.TrimSpace(a.paletteQuery)
	add := func(item paletteItem, text string) {
		if score := matchScore(text, q); score >= 0 {
			founds = append(founds, found{item, score})
		}
	}
	for ti, t := range a.tabs {
		panes := t.panes()
		for pi, p := range panes {
			name, detail := p.label()
			text := fmt.Sprintf("%s %s tab %d pane %d", name, detail, ti+1, pi+1)
			keys := fmt.Sprintf(tr("Pane %d"), pi+1)
			if len(a.tabs) > 1 {
				keys = fmt.Sprintf(tr("Tab %d · Pane %d"), ti+1, pi+1)
			}
			ti, t, p := ti, t, p
			add(paletteItem{
				title: name, detail: detail, glyph: paneProgram(p).Glyph,
				keys: keys,
				run: func() {
					a.selectTab(ti)
					t.setFocus(p)
					a.focusReq = p
				},
			}, text)
		}
	}
	for _, cmd := range paletteCommands {
		if cmd.Hidden {
			continue
		}
		cmd := cmd
		add(paletteItem{title: cmd.label(), keys: cmd.keysOf(), glyph: "command", run: func() { cmd.Run(a) }}, cmd.label())
	}
	slices.SortStableFunc(founds, func(x, y found) int { return y.score - x.score })
	items := make([]paletteItem, len(founds))
	for i, f := range founds {
		items[i] = f.item
	}
	return items
}

// found is a row of the palette and how well it matches the query.
type found struct {
	item  paletteItem
	score int
}

// matchScore ranks text for the query, -1 when the query's letters are
// not in it in order: a run of them from a word's start is best, and
// among those, the shorter the text.
func matchScore(text, q string) int {
	s := strings.ToLower(text)
	lq := strings.ToLower(strings.TrimSpace(q))
	if lq == "" {
		return 0
	}
	if i := strings.Index(s, lq); i >= 0 {
		score := 100 - min(i, 50)
		if i == 0 || s[i-1] == ' ' || s[i-1] == ':' {
			score += 40
		}
		return score + max(0, 20-len(s)/6)
	}
	score, at, run, word := 0, 0, 0, true
	for _, r := range s {
		if at < len(lq) && byte(r) == lq[at] {
			at++
			run++
			score += 2 + min(run, 6)
			if word {
				score += 8
			}
			word = false
			continue
		}
		run = 0
		word = r == ' ' || r == ':'
	}
	if at < len(lq) {
		return -1
	}
	return score
}

// palette draws the command palette, over the window.
func (a *App) palette(c *ui.Context, k *colors) {
	if !a.paletteOpen {
		return
	}
	items := a.paletteItems()
	a.paletteSel = min(max(a.paletteSel, 0), max(len(items)-1, 0))
	was := a.paletteOpen
	// reveal scrolls the selection into view in the frame the keys or the
	// query move it, and only then, so that the wheel scrolls the list.
	reveal := false
	ui.DialogBase(c, &a.paletteOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(k.backdrop).Justify(ui.Start).Padding(78, 0, 0, 0)
		panel.Width(560).MaxHeight(474).Radius(20).Material(panelGlass(k)).Border(1, k.panelBorder).
			Shadow(0, 24, 60, -8, ui.RGBA(0, 0, 0, 0.28)).Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.06)).Clip()
		ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(k.panelBorder).Children(func() {
			ui.Icon(c, icon("search")).Size(17, 17).TextColor(k.textFaint)
			in := ui.TextInputBase(c, &a.paletteQuery).Grow(1).FontSize(15).Placeholder(tr("Type a command, or the name of a pane…")).AutoFocus()
			if in.Changed() {
				a.paletteSel, reveal = 0, true
			}
			if in.Shortcut(0, ui.KeyDown) {
				a.paletteSel, reveal = min(a.paletteSel+1, len(items)-1), true
			}
			if in.Shortcut(0, ui.KeyUp) {
				a.paletteSel, reveal = max(a.paletteSel-1, 0), true
			}
			if in.Submitted() && len(items) > 0 {
				a.paletteOpen = false
				items[a.paletteSel].run()
			}
		})
		listH := float32(min(max(len(items), 1)*34+12, 384))
		ui.Scroll(c).Height(listH).Padding(6).Children(func() {
			if len(items) == 0 {
				ui.Text(c, tr("No matches")).FontSize(13).TextColor(k.textFaint).Padding(14)
			}
			for i, it := range items {
				row := ui.Row(c.Key(i)).Height(34).Padding(0, 10).Gap(10).Radius(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if i == a.paletteSel {
					row.Background(k.panelSel)
					if reveal {
						row.ScrollIntoView()
					}
				} else if row.Hovered() {
					row.Background(k.hover)
				}
				if row.Clicked() {
					a.paletteOpen = false
					it.run()
				}
				row.Children(func() {
					ui.Icon(c, icon(it.glyph)).Size(15, 15).TextColor(k.textMuted)
					ui.Text(c, it.title).FontSize(13.5).FontWeight(500).TextColor(k.text).SingleLine().Ellipsis("…").Shrink(0.3).MinWidth(0)
					if it.detail != "" {
						ui.Text(c, it.detail).FontSize(13).TextColor(k.textFaint).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
					}
					ui.Spacer(c)
					if it.keys != "" {
						ui.Text(c, it.keys).FontSize(12).TextColor(k.textFaint).Padding(2, 7).Radius(6).Background(k.hover).Shrink(0)
					}
				})
			}
		})
		ui.Row(c).Padding(9, 16).Gap(14).AlignItems(ui.Center).BorderWidth(0.5, 0, 0, 0).BorderColor(k.panelBorder).Children(func() {
			hint := func(keys, label string) {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Text(c, keys).FontSize(11).TextColor(k.textFaint).Padding(1, 6).Radius(5).Background(k.hover)
					ui.Text(c, label).FontSize(11.5).TextColor(k.textFaint)
				})
			}
			hint("↑↓", tr("Navigate"))
			hint("↵", tr("Open"))
			hint("Esc", tr("Close"))
		})
	})
	if was && !a.paletteOpen {
		if t := a.tab(); t != nil && a.focusReq == nil {
			a.focusReq = t.Focus
		}
	}
}
