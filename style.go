package main

import (
	_ "embed"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// colors are the app's own, in light and dark windows.
type colors struct {
	// The window's background: a gradient from top through mid to bottom,
	// with two glows for the glass to bend.
	bgTop, bgMid, bgLow, bgBottom ui.Color
	glowA, glowB                  ui.Color

	// Tints of the glass: the panes', the title bar widgets', and the
	// floating panels'. A tint's alpha keeps some of the frost showing.
	glassTint, glassTintFocused, chromeTint, panelTint ui.Color

	cardBorder, cardBorderFocused ui.Color
	shadow, shadowFocused         ui.Color

	text, textMuted, textFaint, iconMuted ui.Color
	hover, pressed                        ui.Color
	raise, facePress                      ui.Color
	focus                                 ui.Color

	track, trackBorder, tabActive, tabSep ui.Color
	tileRim                               ui.Color

	busy, attention, exited ui.Color

	panel, panelBorder, panelSel, backdrop ui.Color
}

var lightColors = colors{
	bgTop:    ui.Hex("#c3d4f4"),
	bgMid:    ui.Hex("#ccd9ec"),
	bgLow:    ui.Hex("#d6e2de"),
	bgBottom: ui.Hex("#e9e5c6"),
	glowA:    ui.RGBA(79, 125, 255, 0.50),
	glowB:    ui.RGBA(53, 194, 186, 0.42),

	glassTint:        ui.RGBA(255, 255, 255, 0.26),
	glassTintFocused: ui.RGBA(255, 255, 255, 0.46),
	chromeTint:       ui.RGBA(255, 255, 255, 0.36),
	panelTint:        ui.RGBA(255, 255, 255, 0.70),

	cardBorder:        ui.RGBA(255, 255, 255, 0.72),
	cardBorderFocused: ui.RGBA(255, 255, 255, 1),
	shadow:            ui.RGBA(60, 40, 50, 0.07),
	shadowFocused:     ui.RGBA(60, 40, 50, 0.12),

	text:      ui.Hex("#1d1d1f"),
	textMuted: ui.Hex("#636366"),
	textFaint: ui.Hex("#8e8e93"),
	iconMuted: ui.Hex("#8a8a8f"),
	hover:     ui.RGBA(0, 0, 0, 0.055),
	pressed:   ui.RGBA(0, 0, 0, 0.1),
	raise:     ui.RGBA(255, 255, 255, 0.62),
	facePress: ui.RGBA(0, 0, 0, 0.14),
	focus:     ui.RGBA(46, 111, 208, 0.5),

	track:       ui.RGBA(255, 255, 255, 0.34),
	trackBorder: ui.RGBA(255, 255, 255, 0.45),
	tabActive:   ui.RGBA(255, 255, 255, 0.97),
	tabSep:      ui.RGBA(0, 0, 0, 0.13),
	tileRim:     ui.RGBA(255, 255, 255, 0.95),

	busy:      ui.Hex("#34a853"),
	attention: ui.Hex("#f59e0b"),
	exited:    ui.Hex("#a1a1a6"),

	panel:       ui.RGBA(255, 255, 255, 0.70),
	panelBorder: ui.RGBA(255, 255, 255, 0.65),
	panelSel:    ui.RGBA(46, 111, 208, 0.12),
	backdrop:    ui.RGBA(50, 25, 50, 0.10),
}

var darkColors = colors{
	bgTop:    ui.Hex("#2b2049"),
	bgMid:    ui.Hex("#262033"),
	bgLow:    ui.Hex("#22212b"),
	bgBottom: ui.Hex("#2a241a"),
	glowA:    ui.RGBA(130, 96, 255, 0.30),
	glowB:    ui.RGBA(42, 190, 180, 0.22),

	glassTint:        ui.RGBA(22, 22, 30, 0.40),
	glassTintFocused: ui.RGBA(22, 22, 30, 0.62),
	chromeTint:       ui.RGBA(22, 22, 30, 0.46),
	panelTint:        ui.RGBA(20, 20, 26, 0.74),

	cardBorder:        ui.RGBA(255, 255, 255, 0.05),
	cardBorderFocused: ui.RGBA(255, 255, 255, 0.22),
	shadow:            ui.RGBA(0, 0, 0, 0.25),
	shadowFocused:     ui.RGBA(0, 0, 0, 0.4),

	text:      ui.Hex("#f2f2f7"),
	textMuted: ui.Hex("#aeaeb2"),
	textFaint: ui.Hex("#8e8e93"),
	iconMuted: ui.Hex("#98989d"),
	hover:     ui.RGBA(255, 255, 255, 0.08),
	pressed:   ui.RGBA(255, 255, 255, 0.14),
	raise:     ui.RGBA(255, 255, 255, 0.11),
	facePress: ui.RGBA(255, 255, 255, 0.18),
	focus:     ui.RGBA(108, 178, 255, 0.55),

	track:       ui.RGBA(255, 255, 255, 0.06),
	trackBorder: ui.RGBA(255, 255, 255, 0.06),
	tabActive:   ui.RGBA(255, 255, 255, 0.14),
	tabSep:      ui.RGBA(255, 255, 255, 0.14),
	tileRim:     ui.RGBA(255, 255, 255, 0.22),

	busy:      ui.Hex("#4ade80"),
	attention: ui.Hex("#fbbf24"),
	exited:    ui.Hex("#636366"),

	panel:       ui.RGBA(24, 24, 30, 0.72),
	panelBorder: ui.RGBA(255, 255, 255, 0.14),
	panelSel:    ui.RGBA(108, 178, 255, 0.18),
	backdrop:    ui.RGBA(0, 0, 0, 0.28),
}

func colorsOf(c *ui.Context) *colors {
	if c.Theme().Dark {
		return &darkColors
	}
	return &lightColors
}

// paneGlass is a pane's card: liquid glass, brighter while focused.
func paneGlass(k *colors, focused bool) glass.Glass {
	tint := k.glassTint
	if focused {
		tint = k.glassTintFocused
	}
	return glass.Glass{Tint: tint}
}

// chromeGlass fills the title bar's widgets: the tab track and the chips.
func chromeGlass(k *colors) glass.Glass {
	return glass.Glass{Tint: k.chromeTint}
}

// panelGlass fills the panels that float over the app: the palette, the
// popovers and the toasts.
func panelGlass(k *colors) glass.Glass {
	return glass.Glass{Tint: k.panelTint}
}

// paintBackground paints the window's background.
func paintBackground(p *ui.Painter, r ui.Rect, k *colors) {
	h1 := r.H * 0.38
	p.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: h1 + 1}, ui.LinearGradient{From: k.bgTop, To: k.bgMid, Angle: 180}, 0)
	p.FillGradient(ui.Rect{X: r.X, Y: r.Y + h1, W: r.W, H: r.H - h1}, ui.LinearGradient{From: k.bgMid, To: k.bgBottom, Angle: 180}, 0)
	// A violet glow from the top right, and a teal one from the bottom
	// left, which the glass bends along its rims.
	p.FillGradient(r, ui.LinearGradient{From: k.glowA, To: k.glowA.Alpha(0), Angle: 135, Start: 0, End: 0.6}, 0)
	p.FillGradient(r, ui.LinearGradient{From: k.glowB, To: k.glowB.Alpha(0), Angle: 315, Start: 0, End: 0.55}, 0)
}

//go:embed assets/fonts/JetBrainsMonoNerdFontMono-Regular.ttf
var fontRegular []byte

//go:embed assets/fonts/JetBrainsMonoNerdFontMono-Bold.ttf
var fontBold []byte

//go:embed assets/fonts/JetBrainsMonoNerdFontMono-Italic.ttf
var fontItalic []byte

//go:embed assets/fonts/JetBrainsMonoNerdFontMono-BoldItalic.ttf
var fontBoldItalic []byte

//go:embed assets/fonts/JetBrainsMonoNLNerdFontMono-Regular.ttf
var fontNLRegular []byte

//go:embed assets/fonts/JetBrainsMonoNLNerdFontMono-Bold.ttf
var fontNLBold []byte

//go:embed assets/fonts/JetBrainsMonoNLNerdFontMono-Italic.ttf
var fontNLItalic []byte

//go:embed assets/fonts/JetBrainsMonoNLNerdFontMono-BoldItalic.ttf
var fontNLBoldItalic []byte

func registerFonts() {
	for _, f := range [][]byte{fontRegular, fontBold, fontItalic, fontBoldItalic} {
		ui.RegisterFont(f, "JetBrainsMono Nerd Font Mono")
	}
	for _, f := range [][]byte{fontNLRegular, fontNLBold, fontNLItalic, fontNLBoldItalic} {
		ui.RegisterFont(f, defaultFontNLFamily)
	}
}

var termFont = terminal.Font{Family: terminalFontFamily(defaultFontFamily), Size: defaultFontSize, LineHeight: 1.0}

// The terminals' colors: soft ink on the panes' paper, and the dark kind.
var lightTerm = &terminal.Theme{
	Foreground: ui.Hex("#2a2d31"),
	Background: ui.Hex("#fbfbfb"),
	Cursor:     ui.Hex("#3a3d42"),
	Selection:  ui.RGBA(46, 111, 208, 0.2),
	Palette: [16]ui.Color{
		ui.Hex("#2a2d31"), ui.Hex("#c2465a"), ui.Hex("#4e8e5f"), ui.Hex("#b07a1e"),
		ui.Hex("#2e6fd0"), ui.Hex("#9050c8"), ui.Hex("#1e8c9c"), ui.Hex("#8a8d93"),
		ui.Hex("#6e7178"), ui.Hex("#d9566b"), ui.Hex("#5ba671"), ui.Hex("#c89026"),
		ui.Hex("#4a8fe0"), ui.Hex("#a56bd8"), ui.Hex("#2ba2b3"), ui.Hex("#b5b8be"),
	},
}

var darkTerm = &terminal.Theme{
	Foreground: ui.Hex("#e6e6ea"),
	Background: ui.Hex("#1e1e22"),
	Cursor:     ui.Hex("#e6e6ea"),
	Selection:  ui.RGBA(108, 178, 255, 0.28),
	Palette: [16]ui.Color{
		ui.Hex("#3a3a40"), ui.Hex("#ff6b7f"), ui.Hex("#7bd88f"), ui.Hex("#e5c07b"),
		ui.Hex("#6cb2ff"), ui.Hex("#c792ea"), ui.Hex("#56d4dd"), ui.Hex("#d0d0d6"),
		ui.Hex("#6e6e78"), ui.Hex("#ff8c9c"), ui.Hex("#9be8a8"), ui.Hex("#f2d28a"),
		ui.Hex("#8ec5ff"), ui.Hex("#d7a8f2"), ui.Hex("#7fe3ea"), ui.Hex("#ffffff"),
	},
}
