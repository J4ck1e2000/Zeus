package shellintegration

import (
	"slices"
	"testing"
)

func TestHasNonInteractiveCommand(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: []string{"bash"}, want: false},
		{args: []string{"bash", "-l"}, want: false},
		{args: []string{"bash", "-c", "ls"}, want: true},
		{args: []string{"bash", "--command", "ls"}, want: true},
		{args: []string{"bash", "-s"}, want: true},
		{args: []string{"bash", "--"}, want: true},
		{args: []string{"bash", "--norc"}, want: true},
		{args: []string{"bash", "--noprofile"}, want: true},
		{args: []string{"bash", "--init-file", "/etc/motd"}, want: true},
		{args: []string{"bash", "--init-file=/etc/motd"}, want: true},
		{args: []string{"bash", "-l", "-c", "ls"}, want: true},
	}
	for _, test := range tests {
		if got := hasNonInteractiveCommand(test.args); got != test.want {
			t.Errorf("hasNonInteractiveCommand(%q) = %v, want %v", test.args, got, test.want)
		}
	}
}

func TestUniqueOverrides(t *testing.T) {
	got := uniqueOverrides(map[string]string{"B": "2", "A": "1", "C": "3"})
	want := []string{"A=1", "B=2", "C=3"}
	if !slices.Equal(got, want) {
		t.Errorf("uniqueOverrides = %q, want %q", got, want)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		executable string
		want       Shell
	}{
		{executable: "/bin/bash", want: Bash},
		{executable: "/usr/local/bin/zsh", want: Zsh},
		{executable: "/usr/bin/fish", want: Fish},
		{executable: "/bin/sh", want: Unknown},
		{executable: "/usr/bin/false", want: Unknown},
	}
	for _, test := range tests {
		if got := Detect(test.executable); got != test.want {
			t.Errorf("Detect(%q) = %v, want %v", test.executable, got, test.want)
		}
	}
}
