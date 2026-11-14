package main

import (
	"slices"
	"strings"
	"testing"
)

func TestSanitizeSettings(t *testing.T) {
	t.Run("new tab start", func(t *testing.T) {
		tests := []struct {
			start, dir string
			want       string
		}{
			// The folder the user chose survives a restart: this is the
			// load-time bug the review found.
			{start: "choose", dir: "/tmp/somewhere", want: "choose"},
			{start: "choose", dir: "", want: "current"},
			{start: "home", want: "home"},
			{start: "current", want: "current"},
			{start: "bogus", want: "current"},
			{start: "", want: "current"},
		}
		for _, test := range tests {
			p := settings{NewTabStart: test.start, NewTabDir: test.dir}
			sanitizeSettings(&p)
			if p.NewTabStart != test.want {
				t.Errorf("NewTabStart %q with dir %q lands on %q, want %q", test.start, test.dir, p.NewTabStart, test.want)
			}
		}
	})

	t.Run("font size", func(t *testing.T) {
		tests := []struct {
			in   float32
			want float32
		}{
			{in: 0, want: defaultFontSize}, // unset, not tiny
			{in: 16, want: 16},
			{in: 5, want: 8},
			{in: 50, want: 32},
		}
		for _, test := range tests {
			p := settings{FontSize: test.in}
			sanitizeSettings(&p)
			if p.FontSize != test.want {
				t.Errorf("FontSize %v lands on %v, want %v", test.in, p.FontSize, test.want)
			}
		}
	})

	t.Run("scrollback", func(t *testing.T) {
		tests := []struct {
			in   int
			want int
		}{
			{in: 0, want: 10},
			{in: 10, want: 10},
			{in: 5000, want: 1024}, // a hand-edited file cannot ask for 5 GiB
			{in: -3, want: -1},
			{in: -1, want: -1},
		}
		for _, test := range tests {
			p := settings{ScrollbackMB: test.in}
			sanitizeSettings(&p)
			if p.ScrollbackMB != test.want {
				t.Errorf("ScrollbackMB %d lands on %d, want %d", test.in, p.ScrollbackMB, test.want)
			}
		}
	})

	t.Run("shortcuts", func(t *testing.T) {
		p := settings{Shortcuts: map[string]shortcutSetting{
			"New Tab": {Mods: 4, Key: 20},
			"No key":  {Mods: 4, Key: 0},
			"No mods": {Mods: 0, Key: 20},
			"Empty":   {},
		}}
		sanitizeSettings(&p)
		if len(p.Shortcuts) != 1 || p.Shortcuts["New Tab"].Key != 20 {
			t.Errorf("the shortcuts are %v, want only the valid one kept", p.Shortcuts)
		}
		// A file with none at all is fine.
		p = settings{}
		sanitizeSettings(&p)
	})

	t.Run("choices", func(t *testing.T) {
		p := settings{Cursor: "bogus", Language: "fr", Appearance: "nonsense"}
		sanitizeSettings(&p)
		if p.Cursor != "" || p.Language != "" {
			t.Errorf("the cursor and language are %q and %q, want empty", p.Cursor, p.Language)
		}
	})
}

func TestTerminalFontFamily(t *testing.T) {
	families := splitFamilies(terminalFontFamily("SF Mono"))
	if families[0] != "SF Mono" {
		t.Errorf("the chosen family is not first: %q", families)
	}
	for _, want := range []string{defaultFontFamily, "Menlo", "monospace"} {
		if !slices.Contains(families, want) {
			t.Errorf("%q is missing from %q", want, families)
		}
	}
	if families := splitFamilies(terminalFontFamily("")); !slices.Contains(families, defaultFontFamily) {
		t.Errorf("an empty choice falls back, but %q lacks %q", families, defaultFontFamily)
	}
}

// splitFamilies splits a comma-separated family list the way
// terminalFontFamily builds it.
func splitFamilies(list string) []string {
	families := []string{}
	for _, family := range strings.Split(list, ",") {
		family = strings.TrimSpace(strings.Trim(family, `"'`))
		if family != "" {
			families = append(families, family)
		}
	}
	return families
}
