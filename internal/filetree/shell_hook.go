package filetree

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"zeus/internal/shellintegration"
)

type ShellHookManager interface {
	Probe(context.Context, SSHTarget) (RemoteShellInfo, error)
	Install(context.Context, SSHTarget, RemoteShellInfo) error
	Uninstall(context.Context, SSHTarget, RemoteShellInfo) error
}

type sshShellHookManager struct {
	probe sshShellHookProbe
}

func NewShellHookManager() ShellHookManager {
	return sshShellHookManager{}
}

func NewShellHookManagerWithRunner(runner SSHCommandRunner) ShellHookManager {
	return sshShellHookManager{probe: sshShellHookProbe{Runner: runner}}
}

func (m sshShellHookManager) Probe(ctx context.Context, target SSHTarget) (RemoteShellInfo, error) {
	return m.probe.Probe(ctx, target)
}

func (m sshShellHookManager) Install(ctx context.Context, target SSHTarget, confirmed RemoteShellInfo) error {
	current, err := m.probe.Probe(ctx, target)
	if err != nil {
		return err
	}
	if !sameRemoteShell(current, confirmed) {
		return fmt.Errorf("remote shell changed after setup probe; check the target again")
	}
	if current.Installed {
		return nil
	}
	hook, err := shellintegration.RemoteHook(current.Shell)
	if err != nil {
		return err
	}
	script, err := remoteShellInstaller(current, hook, false)
	if err != nil {
		return err
	}
	if err := m.runInstaller(ctx, target, script); err != nil {
		return err
	}
	installed, err := m.probe.Probe(ctx, target)
	if err != nil {
		return err
	}
	if !installed.Installed {
		return fmt.Errorf("SSH %s: shell hook installer returned success but the hook is not installed", target.Destination)
	}
	return nil
}

func (m sshShellHookManager) Uninstall(ctx context.Context, target SSHTarget, confirmed RemoteShellInfo) error {
	current, err := m.probe.Probe(ctx, target)
	if err != nil {
		return err
	}
	if !sameRemoteShell(current, confirmed) {
		return fmt.Errorf("remote shell changed after setup probe; check the target again")
	}
	hook, err := shellintegration.RemoteHook(current.Shell)
	if err != nil {
		return err
	}
	script, err := remoteShellInstaller(current, hook, true)
	if err != nil {
		return err
	}
	if err := m.runInstaller(ctx, target, script); err != nil {
		return err
	}
	remaining, err := m.probe.Probe(ctx, target)
	if err != nil {
		return err
	}
	if remaining.Installed {
		return fmt.Errorf("SSH %s: shell hook uninstall returned success but the hook is still installed", target.Destination)
	}
	return nil
}

func (m sshShellHookManager) runInstaller(ctx context.Context, target SSHTarget, script string) error {
	commandCtx, cancel := context.WithTimeout(ctx, m.probe.timeout())
	defer cancel()
	_, stderr, err := m.probe.runner()(commandCtx, m.probe.executable(), target.WorkingDir, m.probe.sshArgs(target, "sh -s"), script)
	if err != nil {
		return sshDiagnostic("update remote shell integration", target.Destination, stderr, err)
	}
	return nil
}

func sameRemoteShell(left, right RemoteShellInfo) bool {
	return left.Host == right.Host && left.User == right.User && left.Shell == right.Shell && left.Home == right.Home && slices.Equal(left.StartupFiles, right.StartupFiles)
}

func validateRemoteShellInfo(info RemoteShellInfo) error {
	if info.Shell != shellintegration.Bash && info.Shell != shellintegration.Zsh && info.Shell != shellintegration.Fish {
		return fmt.Errorf("%w: unsupported login shell", ErrRemoteShellUnsupported)
	}
	if info.Home == "" || strings.ContainsAny(info.Home, "\x00\r\n\t\x1f\x7f") {
		return fmt.Errorf("%w: remote HOME contains an invalid character", ErrRemoteShellUnsupported)
	}
	if info.Shell == shellintegration.Bash {
		if len(info.StartupFiles) != 2 || !isBashLoginStartupFile(info.Home, info.StartupFiles[0]) || info.StartupFiles[1] != path.Join(info.Home, ".bashrc") {
			return fmt.Errorf("remote Bash startup paths do not match the probed home directory")
		}
		return nil
	}
	want := remoteStartupFiles(info.Shell, info.Home)
	if !slices.Equal(info.StartupFiles, want) {
		return fmt.Errorf("remote startup paths do not match the probed home directory")
	}
	return nil
}

func (m sshShellHookManager) timeout() time.Duration { return m.probe.timeout() }
