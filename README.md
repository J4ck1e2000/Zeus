# Zeus

A replica of [Superlogical's Rex](https://www.superlogical.com/updates/public-testing-beginning)
terminal, written in Go with [MyGo](https://mygo.egoist.dev)'s native UI and its
terminal plugin (Ghostty's libghostty-vt). No webview, no cgo: one ~15 MB app,
dressed in Liquid Glass over an aurora.

This project is based on [egoist/gorex](https://github.com/egoist/gorex).

## What it does

- **Tabs and split panes.** Every pane is a card over the window's gradient,
  with its program's icon, name and directory in its header, and buttons to
  split right, split down, zoom and close. Drag the gaps between panes to
  resize them (double-click resets a split to half).
- **Persistent sessions.** Shells run in a session server, a background
  process of the same binary (`Zeus -server`). Quit Zeus and everything
  keeps running; open it again and every tab, split and pane comes back as
  it was, programs still running. The server keeps each session's screen in
  a headless libghostty-vt emulator and sends a snapshot on attach, so
  full-screen programs (lazygit, vim, htop…) come back exactly.
- **Program activity.** The server watches each terminal's foreground
  process (`tcgetpgrp`, `sysctl`, `proc_pidinfo`; on Windows, the shell's
  newest child, its command line and directory): headers and tabs name the
  program (`Node`, `Git Changes` for lazygit, `Codex`, `SSH host`…) and its
  working directory, a green dot shows a program printing, a hollow ring
  marks one running quietly, and an orange dot marks a pane whose program
  finished or rang the bell out of sight. When the window is in the
  background, a long command finishing posts a notification.
- **Tab icons** stack the tiles of a tab's programs, the focused pane's in
  front, as Rex does. Tabs reorder by dragging, rename by double-click, close
  with a middle click, and have a context menu.
- **Host chip** with the machine's name and model; its popover shows the
  chip, memory, OS and the session server.
- **File tree** follows the focused pane and switches between local and SSH
  directories. Open it from the folder button or the command palette. Folders
  expand in the tree; clicking a file copies its path. For SSH, Zeus uses the
  local OpenSSH `sftp` client, SSH config, and keys or `ssh-agent`; the remote
  host must allow SFTP, but needs no Zeus installation. The remote shell must
  report OSC 7 while SSH is active for Zeus to follow its exact current
  directory. Local shell paths are never reused as remote paths. Without a
  remote OSC 7 update, Zeus marks that directory unknown and browses from
  SFTP's starting directory. New local Bash, Zsh, Fish, and PowerShell panes
  report their current directory automatically without changing local shell
  dotfiles. On an SSH pane, **Manage shell integration** can install a small
  prompt hook for remote Bash, Zsh, or Fish. Zeus shows the remote shell and
  startup files first; installation and removal each require confirmation.
  For remote Bash, setup uses the login startup file Bash already selects and
  creates `.bash_profile` only when no `.bash_profile`, `.bash_login`, or
  `.profile` exists.
  The hook is stored in the remote user's home directory, takes effect after
  reconnecting SSH, and does not install a remote server or background process.
  Zeus uses non-interactive OpenSSH authentication for setup, so password-only
  hosts are unsupported. SSH `RemoteCommand`, forced commands, unsupported
  shells, or unavailable key/agent authentication leave SFTP browsing available
  and report why directory tracking cannot be enabled.
- **Command palette** (⇧⌘P, Ctrl+Shift+P, or the ⌘ button): every command, and every pane
  to jump to, fuzzy-matched and ranked.
- **Find** (⌘F, Ctrl+Shift+F): searches the pane's screen and scrollback,
  counting the matches and jumping between them with ⏎ and ⇧⏎.
- Light and dark appearances, text size (⌘+ ⌘− ⌘0), font and line height, the
  cursor's shape and blinking, how much scrollback sessions keep, copy on
  select, and the interface's language (English or 简体中文, following the
  system's until you choose) — all in Settings, with shortcuts the user can
  change. JetBrainsMono Nerd Font Mono and JetBrainsMonoNL Nerd Font Mono
  are embedded.

## Shortcuts

| macOS | Windows, Linux | |
|---|---|---|
| ⌘T | Ctrl+Shift+T | New tab |
| ⌘D / ⇧⌘D | Ctrl+Shift+D / Ctrl+Shift+E | Split right / down |
| ⌘W / ⇧⌘W | Ctrl+Shift+W | Close pane / tab |
| ⇧⌘↩ | Ctrl+Shift+Enter | Zoom the pane |
| ⌥⌘ arrows | Alt+arrows | Focus the pane in that direction |
| ⌃⌘ arrows | Alt+Shift+arrows | Move the nearest divider |
| ⌃⌘= | | Equalize panes |
| ⌘1…⌘9, ⇧⌘[ ⇧⌘], ⌃Tab | Ctrl+Shift+1…9, Ctrl+PageUp/PageDown, Ctrl+Tab | Switch tabs |
| ⇧⌘R | Ctrl+Shift+R | Rename tab |
| ⇧⌘P, ⌘P | Ctrl+Shift+P | Command palette |
| ⌘F | Ctrl+Shift+F | Find in the pane |
| ⌘K | Ctrl+Shift+K | Clear |
| ⌘+ ⌘− ⌘0 | Ctrl+= Ctrl+- Ctrl+0 | Text size |
| ⌥⌘Q | | Quit and end all sessions |

Elsewhere than on macOS, Control and a letter are the terminal's (Ctrl+C,
Ctrl+D, Ctrl+W…), so Zeus's shortcuts take Shift too, as copying and
pasting do (Ctrl+Shift+C, Ctrl+Shift+V), and none take Control and Alt,
which type characters (AltGr) on many keyboards.

## Develop and build

Zeus needs [Go](https://go.dev/dl/) 1.27+ and MyGo 0.3.0, whose CLI
`go.mod` pins as a tool:

```sh
go tool mygo dev     # Zeus Dev, rebuilt and restarted as the code changes
go tool mygo build   # build/darwin-arm64/Zeus.app and a .dmg
go run ./tools/mkicon  # redraw resources/icon.png
```

`go get -tool github.com/egoist/mygo/cmd/mygo@latest` updates MyGo and its
CLI together.

The vendored MyGo carries local patches, which regenerating the vendor
directory drops: see `patches/README.md` to restore them.

The app keeps its state in its data directory: the server's socket and
log, `layout.json` and `settings.json`, in `~/Library/Application
Support/Zeus` for the built app and `Zeus Dev` for `mygo dev`'s, so
that developing never touches the sessions of the app you use
(`ZEUS_DIR` names another directory). The sessions outlive the app, but
not a rebuild: a development build replaces a server that an older build
started, ending its sessions, which then start again in the same
directories.

### Windows

Zeus runs on Windows 10 1809 or later, whose ConPTY runs the shells; its
data directory is `%AppData%\Zeus`. Panes run `$SHELL` when it names a
Windows program (`nu`, `C:\Program Files\Git\bin\bash.exe`), else
PowerShell (`pwsh`, then `powershell`), else `cmd`. PowerShell's `cd`
leaves its process's directory where it was, so Zeus starts it with a
prompt that sets it, wrapping the profile's own prompt: the header then
follows it, and new panes open there. The window is frameless and its
title bar draws minimize, maximize and close with the other buttons; it
resizes from its sides and bottom, not its top.

## Layout

| | |
|---|---|
| `main.go` | the app, its window, and `-server` |
| `state.go` | tabs, the tree of splits, panes, saving and restoring the layout |
| `view.go`, `tabs.go`, `host.go`, `commands.go` | the interface: title bar, tabs, panes, host popover, menus and palette |
| `file_tree.go`, `file_tree_shell_hook.go`, `internal/filetree`, `internal/shellintegration` | the pane-following file tree, local shell hooks, and explicit SSH shell-hook setup |
| `programs.go` | how programs show: names, glyphs, tile colors |
| `style.go`, `settings.go` | colors, fonts, terminal themes; appearance and text size |
| `internal/zeus` | the session server and its client: PTYs, foreground processes, headless screens, the socket protocol |

Zeus does not manage remote terminal sessions; its session server listens on
a local socket. The file tree can still browse a direct SSH session through
the local OpenSSH SFTP client.

Icons: [Lucide](https://lucide.dev) (ISC) and [Simple Icons](https://simpleicons.org)
(CC0). Fonts: [JetBrains Mono](https://www.jetbrains.com/lp/mono/) and
[Nerd Fonts](https://github.com/ryanoasis/nerd-fonts) (OFL).
