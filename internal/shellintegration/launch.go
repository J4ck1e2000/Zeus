package shellintegration

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type File struct {
	Name    string
	Content []byte
	Mode    fs.FileMode
}

type LocalOptions struct {
	Executable     string
	Args           []string
	IntegrationDir string
	Environment    []string
}

type Launch struct {
	Path  string
	Args  []string
	Env   []string
	Files []File
}

func PrepareLocal(shell Shell, options LocalOptions) (Launch, error) {
	launch := Launch{Path: options.Executable, Args: append([]string(nil), options.Args...)}
	if shell == Unknown {
		return launch, nil
	}
	if options.IntegrationDir == "" {
		return Launch{}, fmt.Errorf("shell integration directory is empty")
	}

	switch shell {
	case PowerShell:
		if hasPowerShellCommand(options.Args) {
			return launch, nil
		}
		if !containsFold(launch.Args, "-NoExit") {
			launch.Args = append(launch.Args, "-NoExit")
		}
		launch.Args = append(launch.Args, "-Command", PowerShellPromptCommand())
	case Bash:
		if hasNonInteractiveCommand(options.Args) {
			return launch, nil
		}
		hook, err := RemoteHook(Bash)
		if err != nil {
			return Launch{}, err
		}
		args, login := prepareBashArgs(options.Args)
		hookPath := filepath.Join(options.IntegrationDir, "bash-hook.sh")
		initPath := filepath.Join(options.IntegrationDir, "bash-init.sh")
		launch.Args = append(args, "--init-file", initPath)
		launch.Env = uniqueOverrides(options.Environment, map[string]string{
			"ZEUS_BASH_HOOK":   hookPath,
			"ZEUS_SHELL_LOGIN": boolEnv(login),
		})
		launch.Files = []File{
			{Name: "bash-hook.sh", Content: hook.Content, Mode: 0o600},
			{Name: "bash-init.sh", Mode: 0o600, Content: []byte(bashInitFile)},
		}
	case Zsh:
		if hasNonInteractiveCommand(options.Args) {
			return launch, nil
		}
		hook, err := RemoteHook(Zsh)
		if err != nil {
			return Launch{}, err
		}
		wrapperDir := filepath.Join(options.IntegrationDir, "zsh")
		originalZdotdir := envValue(options.Environment, "ZDOTDIR")
		if originalZdotdir == "" {
			originalZdotdir = envValue(options.Environment, "HOME")
		}
		hookPath := filepath.Join(options.IntegrationDir, "zsh-hook.zsh")
		launch.Env = uniqueOverrides(options.Environment, map[string]string{
			"ZEUS_ORIGINAL_ZDOTDIR": originalZdotdir,
			"ZEUS_ZSH_HOOK":         hookPath,
			"ZEUS_ZSH_WRAPPER":      wrapperDir,
			"ZDOTDIR":               wrapperDir,
		})
		launch.Files = []File{
			{Name: "zsh/.zshenv", Mode: 0o600, Content: []byte(zshStartupFile(".zshenv"))},
			{Name: "zsh/.zprofile", Mode: 0o600, Content: []byte(zshStartupFile(".zprofile"))},
			{Name: "zsh/.zshrc", Mode: 0o600, Content: []byte(zshStartupFile(".zshrc"))},
			{Name: "zsh/.zlogin", Mode: 0o600, Content: []byte(zshStartupFile(".zlogin"))},
			{Name: "zsh-hook.zsh", Content: hook.Content, Mode: 0o600},
		}
	case Fish:
		if hasNonInteractiveCommand(options.Args) {
			return launch, nil
		}
		hook, err := RemoteHook(Fish)
		if err != nil {
			return Launch{}, err
		}
		hookPath := filepath.Join(options.IntegrationDir, "fish-init.fish")
		launch.Args = append(launch.Args, "--init-command", "source "+quoteFish(hookPath))
		launch.Files = []File{{Name: "fish-init.fish", Content: hook.Content, Mode: 0o600}}
	default:
		return launch, nil
	}
	return launch, nil
}

func prepareBashArgs(args []string) ([]string, bool) {
	result := append([]string(nil), args...)
	login := false
	if len(result) > 0 && strings.HasPrefix(result[0], "-") && !strings.HasPrefix(result[0], "--") {
		login = true
		result[0] = strings.TrimPrefix(result[0], "-")
	}
	filtered := result[:0]
	for i, arg := range result {
		if i > 0 && (arg == "-l" || arg == "--login") {
			login = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, login
}

func hasNonInteractiveCommand(args []string) bool {
	for _, arg := range args[1:] {
		switch arg {
		case "-c", "--command", "-s", "--":
			return true
		}
	}
	return false
}

func hasPowerShellCommand(args []string) bool {
	for _, arg := range args[1:] {
		switch strings.ToLower(arg) {
		case "-command", "-c", "-file", "-f", "-encodedcommand", "-e":
			return true
		}
	}
	return false
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func uniqueOverrides(environment []string, overrides map[string]string) []string {
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+overrides[key])
	}
	return result
}

func envValue(environment []string, name string) string {
	value := ""
	for _, item := range environment {
		key, v, ok := strings.Cut(item, "=")
		if ok && key == name {
			value = v
		}
	}
	return value
}

func boolEnv(value bool) string {
	if value {
		return "1"
	}
	return ""
}

func quoteFish(value string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + "'"
}

func PowerShellPromptCommand() string {
	return `if (-not $global:__ZeusShellHookLoaded) { $global:__ZeusShellHookLoaded = $true; $global:__ZeusOriginalPrompt = $function:prompt; function global:prompt { $success = $global:?; try { $location = $executionContext.SessionState.Path.CurrentLocation; if ($location.Provider.Name -eq 'FileSystem') { [Environment]::CurrentDirectory = $location.ProviderPath; $uri = [System.Uri]::new($location.ProviderPath).AbsoluteUri; [Console]::Write(([char]27) + "]7;" + $uri + [char]7) } } catch {}; if (-not $success) { Write-Error '' -ErrorAction Ignore }; if ($global:__ZeusOriginalPrompt) { & $global:__ZeusOriginalPrompt } } }`
}

const bashInitFile = `if [ "${ZEUS_SHELL_LOGIN:-}" = "1" ]; then
  [ -r /etc/profile ] && . /etc/profile
  for __zeus_profile in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
    if [ -r "$__zeus_profile" ]; then . "$__zeus_profile"; break; fi
  done
else
  [ -r "$HOME/.bashrc" ] && . "$HOME/.bashrc"
fi
unset __zeus_profile
[ -r "$ZEUS_BASH_HOOK" ] && . "$ZEUS_BASH_HOOK"
`

func zshStartupFile(name string) string {
	var b strings.Builder
	b.WriteString("__zeus_wrapper=${ZEUS_ZSH_WRAPPER:?}\n")
	b.WriteString("__zeus_original=${ZEUS_ORIGINAL_ZDOTDIR:-$HOME}\n")
	b.WriteString("ZDOTDIR=$__zeus_original\n")
	b.WriteString("__zeus_startup=$ZDOTDIR/" + name + "\n")
	b.WriteString("if [[ -r $__zeus_startup ]]; then source \"$__zeus_startup\"; fi\n")
	b.WriteString("ZEUS_ORIGINAL_ZDOTDIR=$ZDOTDIR\n")
	b.WriteString("export ZEUS_ORIGINAL_ZDOTDIR\n")
	b.WriteString("ZDOTDIR=$__zeus_wrapper\n")
	b.WriteString("export ZDOTDIR\n")
	if name == ".zshrc" {
		b.WriteString("[[ -r $ZEUS_ZSH_HOOK ]] && source \"$ZEUS_ZSH_HOOK\"\n")
	}
	b.WriteString("unset __zeus_wrapper __zeus_original __zeus_startup\n")
	return b.String()
}
