package filetree

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"zeus/internal/shellintegration"
)

const defaultShellHookTimeout = 12 * time.Second

var ErrRemoteShellUnsupported = errors.New("remote shell integration is unsupported for this SSH session")

const shellHookStartMarker = "# >>> Zeus shell integration >>>"
const shellHookEndMarker = "# <<< Zeus shell integration <<<"
const shellHookFileStartMarker = "# >>> Zeus shell integration hook >>>"
const shellHookFileEndMarker = "# <<< Zeus shell integration hook <<<"

const remoteShellProbeTemplate = `shell=${SHELL:-}
home=${HOME:-}
name=${shell##*/}
installed=0
probe_marker=__ZEUS_PROBE_MARKER__
base="$home/.config/zeus/shell-integration"
login_startup=
if [ "$name" = bash ]; then
  if [ -f "$home/.bash_profile" ] && [ -r "$home/.bash_profile" ]; then login_startup="$home/.bash_profile"
  elif [ -f "$home/.bash_login" ] && [ -r "$home/.bash_login" ]; then login_startup="$home/.bash_login"
  elif [ -f "$home/.profile" ] && [ -r "$home/.profile" ]; then login_startup="$home/.profile"
  else login_startup="$home/.bash_profile"
  fi
fi
hook_start='# >>> Zeus shell integration hook >>>'
hook_end='# <<< Zeus shell integration hook <<<'
block_start='# >>> Zeus shell integration >>>'
block_end='# <<< Zeus shell integration <<<'
has_managed_file() {
  managed_file=$1
  managed_start=$2
  managed_end=$3
  managed_source=$4
  managed_legacy_source=$5
  [ -f "$managed_file" ] || return 1
  awk -v start="$managed_start" -v end="$managed_end" -v source="$managed_source" -v legacy="$managed_legacy_source" '
    BEGIN { inside=0; bad=0; starts=0; ends=0; sources=0 }
    $0 == start { starts++; if (inside || starts > 1) bad=1; inside=1; next }
    $0 == end { ends++; if (!inside || ends > 1) bad=1; if (source != "" && sources != 1) bad=1; inside=0; next }
    inside && ((source != "" && $0 == source) || (legacy != "" && $0 == legacy)) { sources++ }
    END { if (inside || bad || starts != 1 || ends != 1) exit 1 }
  ' "$managed_file"
}
case "$name" in
  bash)
    hook_file="$base/bash-hook.sh"
    source_line='[ -n "${BASH_VERSION:-}" ] && [ -r "$HOME/.config/zeus/shell-integration/bash-hook.sh" ] && . "$HOME/.config/zeus/shell-integration/bash-hook.sh"'
    legacy_source_line='[ -r "$HOME/.config/zeus/shell-integration/bash-hook.sh" ] && . "$HOME/.config/zeus/shell-integration/bash-hook.sh"'
    if [ ! -L "$hook_file" ] && has_managed_file "$hook_file" "$hook_start" "$hook_end" "" "" && has_managed_file "$login_startup" "$block_start" "$block_end" "$source_line" "$legacy_source_line" && has_managed_file "$home/.bashrc" "$block_start" "$block_end" "$source_line" "$legacy_source_line"; then installed=1; fi
    ;;
  zsh)
    hook_file="$base/zsh-hook.zsh"
    source_line='[ -r "$HOME/.config/zeus/shell-integration/zsh-hook.zsh" ] && . "$HOME/.config/zeus/shell-integration/zsh-hook.zsh"'
    if [ ! -L "$hook_file" ] && has_managed_file "$hook_file" "$hook_start" "$hook_end" "" "" && has_managed_file "$home/.zshrc" "$block_start" "$block_end" "$source_line" ""; then installed=1; fi
    ;;
  fish)
    hook_file="$base/fish-hook.fish"
    source_line='source "$HOME/.config/zeus/shell-integration/fish-hook.fish"'
    if [ ! -L "$hook_file" ] && has_managed_file "$hook_file" "$hook_start" "$hook_end" "" "" && has_managed_file "$home/.config/fish/conf.d/zeus.fish" "$block_start" "$block_end" "$source_line" ""; then installed=1; fi
    ;;
esac
printf '\036%s\037%s\037%s\037%s\037%s\n' "$probe_marker" "$shell" "$home" "$installed" "$login_startup"
`

func remoteShellProbeScript(marker string) string {
	return strings.Replace(remoteShellProbeTemplate, "__ZEUS_PROBE_MARKER__", quotePOSIXUnchecked(marker), 1)
}

type SSHCommandRunner func(context.Context, string, string, []string, string) ([]byte, []byte, error)

type RemoteShellInfo struct {
	Host         string
	User         string
	Shell        shellintegration.Shell
	Home         string
	StartupFiles []string
	Installed    bool
}

type ShellHookProbe interface {
	Probe(context.Context, SSHTarget) (RemoteShellInfo, error)
}

type sshShellHookProbe struct {
	Executable string
	Timeout    time.Duration
	Runner     SSHCommandRunner
}

func NewShellHookProbeWithRunner(runner SSHCommandRunner) ShellHookProbe {
	return sshShellHookProbe{Runner: runner}
}

func (m sshShellHookProbe) Probe(ctx context.Context, target SSHTarget) (RemoteShellInfo, error) {
	var info RemoteShellInfo
	if target.Destination == "" {
		return info, fmt.Errorf("SSH destination is missing")
	}
	commandCtx, cancel := context.WithTimeout(ctx, m.timeout())
	defer cancel()
	runner := m.runner()
	executable := m.executable()
	configOut, configErr, err := runner(commandCtx, executable, target.WorkingDir, m.sshArgs(target, "-G"), "")
	if err != nil {
		return info, sshDiagnostic("inspect SSH config", target.Destination, configErr, err)
	}
	if hasRemoteCommand(configOut) {
		return info, fmt.Errorf("%w: SSH config sets RemoteCommand", ErrRemoteShellUnsupported)
	}
	host, user := parseSSHConfigIdentity(configOut)
	if host == "" || user == "" {
		fallbackHost, fallbackUser := sshDestinationIdentity(target.Destination)
		if host == "" {
			host = fallbackHost
		}
		if user == "" {
			user = fallbackUser
		}
	}
	marker, err := newShellProbeMarker()
	if err != nil {
		return info, fmt.Errorf("create SSH probe marker: %w", err)
	}
	stdout, stderr, err := runner(commandCtx, executable, target.WorkingDir, m.sshArgs(target, "sh -s"), remoteShellProbeScript(marker))
	if err != nil {
		return info, sshDiagnostic("probe remote shell", target.Destination, stderr, err)
	}
	shellPath, home, installed, loginStartup, err := parseShellProbe(stdout, marker)
	if err != nil {
		return info, fmt.Errorf("SSH %s: %w", target.Destination, err)
	}
	shell := shellintegration.Detect(shellPath)
	if shell != shellintegration.Bash && shell != shellintegration.Zsh && shell != shellintegration.Fish {
		return info, fmt.Errorf("%w: unsupported login shell %q", ErrRemoteShellUnsupported, shellPath)
	}
	if !path.IsAbs(home) || strings.ContainsAny(home, "\x00\r\n\x1f\x7f") {
		return info, fmt.Errorf("%w: remote HOME is not an absolute POSIX path", ErrRemoteShellUnsupported)
	}
	startupFiles := remoteStartupFiles(shell, home)
	if shell == shellintegration.Bash {
		if !isBashLoginStartupFile(home, loginStartup) {
			return info, fmt.Errorf("%w: remote Bash login startup file is not recognized", ErrRemoteShellUnsupported)
		}
		startupFiles = remoteBashStartupFiles(home, loginStartup)
	} else if loginStartup != "" {
		return info, fmt.Errorf("%w: non-Bash probe returned a Bash login file", ErrRemoteShellUnsupported)
	}
	return RemoteShellInfo{Host: host, User: user, Shell: shell, Home: home, StartupFiles: startupFiles, Installed: installed}, nil
}

func newShellProbeMarker() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "ZEUS_SHELL_HOOK_PROBE_" + hex.EncodeToString(nonce[:]), nil
}

func parseSSHConfigIdentity(output []byte) (host, user string) {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "hostname":
			host = fields[1]
		case "user":
			user = fields[1]
		}
	}
	return host, user
}

func sshDestinationIdentity(destination string) (host, user string) {
	host = destination
	if index := strings.LastIndex(destination, "@"); index >= 0 {
		user, host = destination[:index], destination[index+1:]
	}
	return host, user
}

func (m sshShellHookProbe) sshArgs(target SSHTarget, remoteCommand string) []string {
	args := []string{"-o", "BatchMode=yes"}
	for i := 0; i < len(target.Options); i++ {
		option := target.Options[i]
		if option == "-P" {
			args = append(args, "-p")
			if i+1 < len(target.Options) {
				i++
				args = append(args, target.Options[i])
			}
			continue
		}
		args = append(args, option)
	}
	if remoteCommand == "-G" {
		args = append(args, "-G")
	}
	if target.DestinationLiteral {
		args = append(args, "--")
	}
	args = append(args, target.Destination)
	if remoteCommand != "" && remoteCommand != "-G" {
		args = append(args, remoteCommand)
	}
	return args
}

func (m sshShellHookProbe) runner() SSHCommandRunner {
	if m.Runner != nil {
		return m.Runner
	}
	return runCommand
}

func (m sshShellHookProbe) executable() string {
	if m.Executable != "" {
		return m.Executable
	}
	return "ssh"
}

func (m sshShellHookProbe) timeout() time.Duration {
	if m.Timeout > 0 {
		return m.Timeout
	}
	return defaultShellHookTimeout
}

func hasRemoteCommand(output []byte) bool {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "remotecommand") {
			return !strings.EqualFold(strings.Join(fields[1:], " "), "none")
		}
	}
	return false
}

func parseShellProbe(output []byte, expectedMarker string) (shell, home string, installed bool, loginStartup string, err error) {
	marker := "\x1e" + expectedMarker + "\x1f"
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		fields := strings.Split(strings.TrimPrefix(line[index:], marker), "\x1f")
		if len(fields) != 4 || fields[0] == "" || fields[1] == "" || (fields[2] != "0" && fields[2] != "1") {
			return "", "", false, "", fmt.Errorf("remote shell probe returned malformed data")
		}
		return fields[0], fields[1], fields[2] == "1", fields[3], nil
	}
	return "", "", false, "", fmt.Errorf("remote shell probe did not return its marker")
}

func remoteStartupFiles(shell shellintegration.Shell, home string) []string {
	switch shell {
	case shellintegration.Bash:
		return []string{path.Join(home, ".bash_profile"), path.Join(home, ".bashrc")}
	case shellintegration.Zsh:
		return []string{path.Join(home, ".zshrc")}
	case shellintegration.Fish:
		return []string{path.Join(home, ".config", "fish", "conf.d", "zeus.fish")}
	default:
		return nil
	}
}

func remoteBashStartupFiles(home, loginStartup string) []string {
	return []string{loginStartup, path.Join(home, ".bashrc")}
}

func isBashLoginStartupFile(home, file string) bool {
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		if file == path.Join(home, name) {
			return true
		}
	}
	return false
}

func sshDiagnostic(action, destination string, stderr []byte, err error) error {
	message := strings.TrimSpace(string(stderr))
	if message != "" {
		return fmt.Errorf("SSH %s %s: %s: %w", action, destination, message, err)
	}
	return fmt.Errorf("SSH %s %s: %w", action, destination, err)
}
