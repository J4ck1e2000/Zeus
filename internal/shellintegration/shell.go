package shellintegration

import (
	"path/filepath"
	"strings"
)

type Shell uint8

const (
	Unknown Shell = iota
	Bash
	Zsh
	Fish
	PowerShell
)

func (s Shell) String() string {
	switch s {
	case Bash:
		return "Bash"
	case Zsh:
		return "Zsh"
	case Fish:
		return "Fish"
	case PowerShell:
		return "PowerShell"
	default:
		return "Unknown"
	}
}

func Detect(executable string) Shell {
	base := executable
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	switch base {
	case "bash":
		return Bash
	case "zsh":
		return Zsh
	case "fish":
		return Fish
	case "pwsh", "powershell":
		return PowerShell
	default:
		return Unknown
	}
}
