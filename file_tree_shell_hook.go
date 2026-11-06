package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"zeus/internal/filetree"
)

type fileTreeShellHookStatus uint8

const (
	fileTreeShellHookIdle fileTreeShellHookStatus = iota
	fileTreeShellHookProbing
	fileTreeShellHookConfirmInstall
	fileTreeShellHookInstalling
	fileTreeShellHookInstalled
	fileTreeShellHookInstalledNeedsReconnect
	fileTreeShellHookConfirmRemove
	fileTreeShellHookUninstalling
	fileTreeShellHookRemoved
	fileTreeShellHookFailed
)

type fileTreeShellHookState struct {
	targetKey  string
	target     filetree.SSHTarget
	info       filetree.RemoteShellInfo
	hasInfo    bool
	status     fileTreeShellHookStatus
	err        string
	cancel     context.CancelFunc
	generation uint64
}

func (s *fileTreeShellHookState) cancelOperation() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *fileTreeShellHookState) resetForTarget(location filetree.Location) {
	target, ok := sshTargetFromFileTreeLocation(location)
	key := ""
	if ok {
		key = fileTreeShellHookTargetKey(target)
	}
	if key == s.targetKey {
		if ok {
			s.target = cloneSSHTarget(target)
		}
		return
	}
	s.cancelOperation()
	s.generation++
	*s = fileTreeShellHookState{generation: s.generation}
	if ok {
		s.targetKey = key
		s.target = cloneSSHTarget(target)
	}
}

func (s *fileTreeShellHookState) syncTarget(location filetree.Location) {
	s.resetForTarget(location)
}

func sshTargetFromFileTreeLocation(location filetree.Location) (filetree.SSHTarget, bool) {
	switch loc := location.(type) {
	case filetree.SSHLocation:
		return loc.Target, true
	case filetree.SSHLocationUnknown:
		return loc.Target, true
	default:
		return filetree.SSHTarget{}, false
	}
}

func fileTreeShellHookTargetKey(target filetree.SSHTarget) string {
	literal := "plain"
	if target.DestinationLiteral {
		literal = "literal"
	}
	return target.Destination + "\x00" + literal + "\x00" + strings.Join(target.Options, "\x00")
}

func cloneSSHTarget(target filetree.SSHTarget) filetree.SSHTarget {
	target.Options = append([]string(nil), target.Options...)
	return target
}

func (a *App) fileTreeShellHookManager() filetree.ShellHookManager {
	if a.shellHookManager == nil {
		a.shellHookManager = filetree.NewShellHookManager()
	}
	return a.shellHookManager
}

func (a *App) probeFileTreeShellHook() {
	s := &a.fileTree.shellHook
	if !a.fileTree.open || s.targetKey == "" || s.status == fileTreeShellHookProbing || s.status == fileTreeShellHookInstalling || s.status == fileTreeShellHookUninstalling {
		return
	}
	target := cloneSSHTarget(s.target)
	targetKey := s.targetKey
	manager := a.fileTreeShellHookManager()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelOperation()
	s.generation++
	generation := s.generation
	s.cancel = cancel
	s.status = fileTreeShellHookProbing
	s.err = ""
	s.hasInfo = false
	win := a.win
	go func() {
		info, err := manager.Probe(ctx, target)
		a.postFileTreeResult(win, func() {
			current := &a.fileTree.shellHook
			if !a.fileTree.open || current.generation != generation || current.targetKey != targetKey {
				return
			}
			current.cancelOperation()
			if err != nil {
				current.status, current.err, current.hasInfo = fileTreeShellHookFailed, err.Error(), false
				return
			}
			current.info = cloneRemoteShellInfo(info)
			current.hasInfo = true
			current.err = ""
			if info.Installed {
				current.status = fileTreeShellHookInstalled
			} else {
				current.status = fileTreeShellHookConfirmInstall
			}
		})
	}()
}

func (a *App) installFileTreeShellHook() {
	s := &a.fileTree.shellHook
	if !a.fileTree.open || s.targetKey == "" || !s.hasInfo || s.status != fileTreeShellHookConfirmInstall {
		return
	}
	a.runFileTreeShellHookChange(fileTreeShellHookInstalling, func(ctx context.Context, manager filetree.ShellHookManager, target filetree.SSHTarget, info filetree.RemoteShellInfo) error {
		return manager.Install(ctx, target, info)
	}, fileTreeShellHookInstalledNeedsReconnect)
}

func (a *App) uninstallFileTreeShellHook() {
	s := &a.fileTree.shellHook
	if !a.fileTree.open || s.targetKey == "" || !s.hasInfo || s.status != fileTreeShellHookConfirmRemove {
		return
	}
	a.runFileTreeShellHookChange(fileTreeShellHookUninstalling, func(ctx context.Context, manager filetree.ShellHookManager, target filetree.SSHTarget, info filetree.RemoteShellInfo) error {
		return manager.Uninstall(ctx, target, info)
	}, fileTreeShellHookRemoved)
}

func (a *App) runFileTreeShellHookChange(
	workingStatus fileTreeShellHookStatus,
	change func(context.Context, filetree.ShellHookManager, filetree.SSHTarget, filetree.RemoteShellInfo) error,
	successStatus fileTreeShellHookStatus,
) {
	s := &a.fileTree.shellHook
	target := cloneSSHTarget(s.target)
	targetKey := s.targetKey
	info := cloneRemoteShellInfo(s.info)
	manager := a.fileTreeShellHookManager()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelOperation()
	s.generation++
	generation := s.generation
	s.cancel = cancel
	s.status = workingStatus
	s.err = ""
	win := a.win
	go func() {
		err := change(ctx, manager, target, info)
		a.postFileTreeResult(win, func() {
			current := &a.fileTree.shellHook
			if !a.fileTree.open || current.generation != generation || current.targetKey != targetKey {
				return
			}
			current.cancelOperation()
			if err != nil {
				current.status, current.err = fileTreeShellHookFailed, err.Error()
				return
			}
			current.info = info
			current.info.Installed = successStatus != fileTreeShellHookRemoved
			current.hasInfo = true
			current.status = successStatus
			current.err = ""
		})
	}()
}

func cloneRemoteShellInfo(info filetree.RemoteShellInfo) filetree.RemoteShellInfo {
	info.StartupFiles = append([]string(nil), info.StartupFiles...)
	return info
}

func (a *App) cancelFileTreeShellHookSetup() {
	s := &a.fileTree.shellHook
	if s.status != fileTreeShellHookConfirmInstall && s.status != fileTreeShellHookFailed {
		return
	}
	s.info = filetree.RemoteShellInfo{}
	s.hasInfo = false
	s.err = ""
	s.status = fileTreeShellHookIdle
}

func (a *App) cancelFileTreeShellHookRemoval() {
	s := &a.fileTree.shellHook
	if s.status == fileTreeShellHookConfirmRemove {
		s.status = fileTreeShellHookInstalled
		s.err = ""
	}
}

func (a *App) fileTreeShellHookControls(c *ui.Context, k *colors) {
	s := &a.fileTree.shellHook
	if s.targetKey == "" {
		return
	}
	ui.Column(c.Key("file tree shell integration")).Gap(5).Children(func() {
		switch s.status {
		case fileTreeShellHookIdle:
			settingsButtonWide(c, k, tr("Manage shell integration"), 210, a.probeFileTreeShellHook)
		case fileTreeShellHookProbing:
			ui.Text(c, tr("Checking remote shell integration…")).FontSize(11.5).TextColor(k.textFaint).Wrap()
		case fileTreeShellHookConfirmInstall:
			a.drawFileTreeShellHookConfirmation(c, k, false)
		case fileTreeShellHookInstalling:
			ui.Text(c, tr("Installing remote shell hook…")).FontSize(11.5).TextColor(k.textFaint).Wrap()
		case fileTreeShellHookInstalled:
			ui.Text(c, tr("Remote shell hook is installed")).FontSize(11.5).TextColor(k.textMuted).Wrap()
			settingsButtonWide(c, k, tr("Remove shell hook"), 150, func() { s.status = fileTreeShellHookConfirmRemove })
		case fileTreeShellHookInstalledNeedsReconnect:
			ui.Text(c, tr("Reconnect SSH pane to apply the hook")).FontSize(11.5).TextColor(k.busy).Wrap()
			settingsButtonWide(c, k, tr("Remove shell hook"), 150, func() { s.status = fileTreeShellHookConfirmRemove })
		case fileTreeShellHookConfirmRemove:
			a.drawFileTreeShellHookConfirmation(c, k, true)
		case fileTreeShellHookUninstalling:
			ui.Text(c, tr("Removing remote shell hook…")).FontSize(11.5).TextColor(k.textFaint).Wrap()
		case fileTreeShellHookRemoved:
			ui.Text(c, tr("Shell hook removed; reconnect SSH to apply")).FontSize(11.5).TextColor(k.textMuted).Wrap()
			settingsButtonWide(c, k, tr("Manage shell integration"), 210, a.probeFileTreeShellHook)
		case fileTreeShellHookFailed:
			ui.Text(c, s.err).FontSize(11.5).TextColor(k.attention).Wrap()
			settingsButtonWide(c, k, tr("Retry shell hook setup"), 190, a.probeFileTreeShellHook)
		}
	})
}

func (a *App) drawFileTreeShellHookConfirmation(c *ui.Context, k *colors, remove bool) {
	s := &a.fileTree.shellHook
	if !s.hasInfo {
		return
	}
	host, user := s.info.Host, s.info.User
	fallbackHost, fallbackUser := shellHookDestinationIdentity(s.target.Destination)
	if host == "" {
		host = fallbackHost
	}
	if user == "" {
		user = fallbackUser
	}
	if user == "" {
		user = tr("Unknown")
	}
	ui.Text(c, fmt.Sprintf(tr("SSH destination: %s"), s.target.Destination)).FontSize(10.5).TextColor(k.textFaint).Wrap()
	ui.Text(c, fmt.Sprintf(tr("Effective SSH host: %s"), host)).FontSize(10.5).TextColor(k.textFaint).Wrap()
	ui.Text(c, fmt.Sprintf(tr("Remote user: %s"), user)).FontSize(10.5).TextColor(k.textFaint).Wrap()
	if remove {
		ui.Text(c, tr("Remove the remote shell hook from these startup files?")).FontSize(11.5).TextColor(k.textMuted).Wrap()
	} else {
		ui.Text(c, fmt.Sprintf(tr("Install a %s hook in these remote startup files?"), s.info.Shell.String())).FontSize(11.5).TextColor(k.textMuted).Wrap()
	}
	for _, startup := range s.info.StartupFiles {
		ui.Text(c, startup).FontSize(10.5).TextColor(k.textFaint).SingleLine().Ellipsis("…").Tooltip(startup)
	}
	ui.Row(c).Gap(5).Children(func() {
		if remove {
			settingsButtonWide(c, k, tr("Confirm remove shell hook"), 145, a.uninstallFileTreeShellHook)
			settingsButtonWide(c, k, tr("Cancel shell hook removal"), 74, a.cancelFileTreeShellHookRemoval)
			return
		}
		settingsButtonWide(c, k, tr("Install shell hook"), 110, a.installFileTreeShellHook)
		settingsButtonWide(c, k, tr("Cancel shell hook setup"), 108, a.cancelFileTreeShellHookSetup)
	})
}

func shellHookDestinationIdentity(destination string) (host, user string) {
	host = destination
	if index := strings.LastIndex(destination, "@"); index >= 0 {
		user, host = destination[:index], destination[index+1:]
	}
	return host, user
}
