package shellintegration

import (
	"embed"
	"fmt"
)

//go:embed hooks/*
var hooks embed.FS

func RemoteHook(shell Shell) (File, error) {
	var path, name string
	switch shell {
	case Bash:
		path, name = "hooks/bash.sh", "bash-hook.sh"
	case Zsh:
		path, name = "hooks/zsh.zsh", "zsh-hook.zsh"
	case Fish:
		path, name = "hooks/fish.fish", "fish-hook.fish"
	default:
		return File{}, fmt.Errorf("remote shell integration is unsupported for shell %v", shell)
	}
	content, err := hooks.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read embedded shell %v hook: %w", shell, err)
	}
	return File{Name: name, Content: content, Mode: 0o600}, nil
}
