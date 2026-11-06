package main

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"zeus/internal/filetree"
)

type fileTreeLoadState uint8

const (
	fileTreeLoading fileTreeLoadState = iota
	fileTreeReady
	fileTreeFailed
)

type fileTreeDirectory struct {
	requested filetree.Location
	resolved  filetree.Location
	state     fileTreeLoadState
	entries   []filetree.Entry
	err       string
	expanded  map[string]bool
}

type fileTreeState struct {
	open           bool
	paneID         int
	selectionKey   string
	sourceLocation filetree.Location
	locationErr    string
	directories    map[string]*fileTreeDirectory
	requests       map[string]context.CancelFunc
	generation     uint64
	copiedPath     string
	copiedAt       time.Time
	shellHook      fileTreeShellHookState
}

func (s *fileTreeState) cancelRequests() {
	for key, cancel := range s.requests {
		cancel()
		delete(s.requests, key)
	}
}

func (s *fileTreeState) reset() {
	s.cancelRequests()
	s.shellHook.resetForTarget(nil)
	s.generation++
	s.open = false
	s.paneID = 0
	s.selectionKey = ""
	s.sourceLocation = nil
	s.locationErr = ""
	s.directories = nil
	s.copiedPath = ""
	s.copiedAt = time.Time{}
}

func (a *App) fileTreeSource() filetree.Source {
	if a.fileSource == nil {
		a.fileSource = filetree.NewSource()
	}
	return a.fileSource
}

func (a *App) toggleFileTree() {
	a.fileTree.open = !a.fileTree.open
	if !a.fileTree.open {
		a.fileTree.cancelRequests()
		a.fileTree.shellHook.resetForTarget(nil)
		a.fileTree.generation++
		a.fileTree.directories = nil
		return
	}
	a.syncFileTree()
}

// syncFileTree binds the sidebar to the active tab's selected pane. It is
// called after the view updates pane focus and after session polling.
func (a *App) syncFileTree() {
	s := &a.fileTree
	t := a.tab()
	if t == nil || t.Focus == nil {
		a.fileTree.shellHook.resetForTarget(nil)
		if s.selectionKey != "" {
			s.cancelRequests()
			s.generation++
			s.selectionKey = ""
			s.paneID = 0
			s.sourceLocation = nil
			s.locationErr = ""
			s.directories = nil
		}
		return
	}

	p := t.Focus
	location, err := filetree.ResolveLocation(p.info)
	if err != nil {
		s.shellHook.resetForTarget(nil)
	} else {
		s.shellHook.syncTarget(location)
	}
	selectionKey := fmt.Sprintf("%d:", p.ID)
	if err != nil {
		selectionKey += "error:" + err.Error()
	} else {
		selectionKey += fileTreeLocationKey(location)
	}
	if selectionKey != s.selectionKey {
		s.cancelRequests()
		s.generation++
		s.paneID = p.ID
		s.selectionKey = selectionKey
		s.sourceLocation = location
		s.locationErr = ""
		s.directories = make(map[string]*fileTreeDirectory)
		if err != nil {
			s.locationErr = err.Error()
		}
	}
	if s.open && s.locationErr == "" && s.sourceLocation != nil {
		a.loadFileTreeDirectory(s.sourceLocation, false)
	}
}

func fileTreeLocationKey(location filetree.Location) string {
	switch loc := location.(type) {
	case filetree.LocalLocation:
		return "local:" + loc.Path
	case filetree.SSHLocation:
		return "ssh:" + sshTargetKey(loc.Target) + ":" + loc.Path
	case filetree.SSHLocationUnknown:
		return "ssh-unknown:" + sshTargetKey(loc.Target)
	default:
		return fmt.Sprintf("unsupported:%T:%#v", location, location)
	}
}

func sshTargetKey(target filetree.SSHTarget) string {
	literal := "plain"
	if target.DestinationLiteral {
		literal = "literal"
	}
	return target.Destination + "\x00" + literal + "\x00" + strings.Join(target.Options, "\x00") + "\x00" + target.WorkingDir
}

func shortFileTreePath(value string, remote bool) string {
	separator := string(filepath.Separator)
	if remote {
		separator = "/"
	}
	if len([]rune(value)) <= 34 {
		return value
	}
	trimmed := strings.Trim(value, separator)
	if trimmed == "" {
		return value
	}
	parts := strings.Split(trimmed, separator)
	if len(parts) >= 2 {
		tail := strings.Join(parts[len(parts)-2:], separator)
		runes := []rune(tail)
		if len(runes) > 30 {
			tail = "…" + string(runes[len(runes)-29:])
		}
		return "…" + separator + tail
	}
	runes := []rune(value)
	return "…" + string(runes[len(runes)-33:])
}

func (a *App) loadFileTreeDirectory(location filetree.Location, force bool) {
	s := &a.fileTree
	if !s.open || location == nil {
		return
	}
	key := fileTreeLocationKey(location)
	if s.directories == nil {
		s.directories = make(map[string]*fileTreeDirectory)
	}
	directory := s.directories[key]
	if directory != nil {
		if directory.state == fileTreeLoading || !force && (directory.state == fileTreeReady || directory.state == fileTreeFailed) {
			return
		}
	} else {
		directory = &fileTreeDirectory{requested: location, expanded: make(map[string]bool)}
		s.directories[key] = directory
	}
	if cancel := s.requests[key]; cancel != nil {
		cancel()
		delete(s.requests, key)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if s.requests == nil {
		s.requests = make(map[string]context.CancelFunc)
	}
	s.requests[key] = cancel
	directory.requested = location
	directory.state = fileTreeLoading
	directory.err = ""

	generation, paneID, selectionKey := s.generation, s.paneID, s.selectionKey
	source, win := a.fileTreeSource(), a.win
	go func() {
		listing, err := source.ReadDir(ctx, location)
		if listing.Location == nil && err == nil {
			listing.Location = location
		}
		a.postFileTreeResult(win, func() {
			current := &a.fileTree
			if current.generation != generation || current.paneID != paneID || current.selectionKey != selectionKey {
				return
			}
			delete(current.requests, key)
			loaded := current.directories[key]
			if loaded == nil || loaded.state != fileTreeLoading {
				return
			}
			if err != nil {
				loaded.state, loaded.err = fileTreeFailed, err.Error()
				return
			}
			loaded.resolved = listing.Location
			loaded.entries = listing.Entries
			loaded.state = fileTreeReady
		})
	}()
}

func (a *App) postFileTreeResult(win *mygo.Window, fn func()) {
	a.post(fn)
	if win != nil {
		win.Update(func() {
			if a.win == win {
				a.runPosted()
			}
		})
	}
}

func (a *App) toggleFileTreeDirectory(parentKey string, entry filetree.Entry) {
	if entry.Kind != filetree.EntryDirectory {
		return
	}
	parent := a.fileTree.directories[parentKey]
	if parent == nil || parent.state != fileTreeReady {
		return
	}
	child, err := fileTreeChildLocation(effectiveFileTreeLocation(parent), entry.Name)
	if err != nil {
		return
	}
	childKey := fileTreeLocationKey(child)
	if parent.expanded[childKey] {
		delete(parent.expanded, childKey)
		return
	}
	parent.expanded[childKey] = true
	a.loadFileTreeDirectory(child, false)
}

func fileTreeChildLocation(parent filetree.Location, name string) (filetree.Location, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, '/') {
		return nil, fmt.Errorf("invalid file name %q", name)
	}
	switch loc := parent.(type) {
	case filetree.LocalLocation:
		return filetree.LocalLocation{Path: filepath.Join(loc.Path, name)}, nil
	case filetree.SSHLocation:
		return filetree.SSHLocation{Target: loc.Target, Path: path.Join(loc.Path, name)}, nil
	default:
		return nil, fmt.Errorf("cannot join file name to %T", parent)
	}
}

func effectiveFileTreeLocation(directory *fileTreeDirectory) filetree.Location {
	if directory.resolved != nil {
		return directory.resolved
	}
	return directory.requested
}

func (a *App) copyFileTreeEntry(c *ui.Context, directory *fileTreeDirectory, name string) {
	pathLocation, err := fileTreeChildLocation(effectiveFileTreeLocation(directory), name)
	if err != nil {
		return
	}
	var text string
	switch loc := pathLocation.(type) {
	case filetree.LocalLocation:
		text = loc.Path
	case filetree.SSHLocation:
		text = loc.Path
	default:
		return
	}
	c.WriteClipboard(text)
	a.fileTree.copiedPath = text
	a.fileTree.copiedAt = time.Now()
}

func (a *App) refreshFileTreeEntry(directory *fileTreeDirectory, entry filetree.Entry) {
	location := effectiveFileTreeLocation(directory)
	if entry.Kind == filetree.EntryDirectory {
		child, err := fileTreeChildLocation(location, entry.Name)
		if err != nil {
			return
		}
		location = child
	}
	a.loadFileTreeDirectory(location, true)
}

func (a *App) fileTreePanel(c *ui.Context, k *colors) ui.Element {
	s := &a.fileTree
	panel := ui.Column(c.Key("file tree panel")).Width(252).MinWidth(190).MaxWidth(320).MinHeight(0).
		Padding(11).Gap(8).Radius(cardR).Material(panelGlass(k)).Border(1, k.panelBorder)
	panel.Children(func() {
		ui.Row(c).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr("Files")).FontSize(13).FontWeight(650).TextColor(k.text)
			ui.Spacer(c)
			if iconButton(c, k, "x", tr("Close File Tree"), 24, 13).Clicked() {
				a.toggleFileTree()
			}
		})
		a.fileTreeLocationHeader(c, k)
		a.fileTreeShellHookControls(c, k)
		ui.Divider(c)
		if s.copiedAt.IsZero() == false {
			remaining := 1200*time.Millisecond - time.Since(s.copiedAt)
			if remaining > 0 {
				c.After(remaining)
				ui.Text(c, tr("Copied path")).FontSize(11.5).TextColor(k.busy).SingleLine().Ellipsis("…")
			}
		}
		ui.Scroll(c.Key("file tree scroll")).Grow(1).MinHeight(0).Children(func() {
			if s.locationErr != "" {
				ui.Text(c, s.locationErr).FontSize(12).TextColor(k.attention).LineHeight(1.35)
				return
			}
			if s.sourceLocation == nil {
				ui.Text(c, tr("No active pane")).FontSize(12).TextColor(k.textFaint)
				return
			}
			a.drawFileTreeDirectory(c, k, fileTreeLocationKey(s.sourceLocation), 0)
		})
	})
	return panel
}

func (a *App) fileTreeLocationHeader(c *ui.Context, k *colors) {
	s := &a.fileTree
	if s.locationErr != "" || s.sourceLocation == nil {
		return
	}
	root := s.directories[fileTreeLocationKey(s.sourceLocation)]
	switch loc := s.sourceLocation.(type) {
	case filetree.LocalLocation:
		ui.Text(c, shortFileTreePath(loc.Path, false)).FontSize(11.5).TextColor(k.textFaint).SingleLine().Ellipsis("…").Tooltip(loc.Path)
	case filetree.SSHLocation:
		current := loc
		if root != nil {
			if resolved, ok := root.resolved.(filetree.SSHLocation); ok {
				current = resolved
			}
		}
		label := current.Target.Destination + ":" + shortFileTreePath(current.Path, true)
		tooltip := current.Target.Destination + ":" + current.Path
		ui.Text(c, label).FontSize(11.5).TextColor(k.textFaint).SingleLine().Ellipsis("…").Tooltip(tooltip)
	case filetree.SSHLocationUnknown:
		ui.Text(c, tr("Remote directory unknown")).FontSize(11.5).TextColor(k.attention).SingleLine().Ellipsis("…")
		ui.Text(c, fmt.Sprintf(tr("Remote host: %s"), loc.Target.Destination)).FontSize(11).TextColor(k.textFaint).SingleLine().Ellipsis("…")
		if root != nil {
			if resolved, ok := root.resolved.(filetree.SSHLocation); ok {
				browsing := fmt.Sprintf(tr("Browsing %s"), shortFileTreePath(resolved.Path, true))
				tooltip := fmt.Sprintf(tr("Browsing %s"), resolved.Path)
				ui.Text(c, browsing).FontSize(11).TextColor(k.textFaint).SingleLine().Ellipsis("…").Tooltip(tooltip)
			}
		}
	}
}

func (a *App) drawFileTreeDirectory(c *ui.Context, k *colors, key string, depth int) {
	directory := a.fileTree.directories[key]
	if directory == nil {
		ui.Text(c, tr("Loading…")).FontSize(12).TextColor(k.textFaint)
		return
	}
	switch directory.state {
	case fileTreeLoading:
		ui.Text(c, tr("Loading…")).FontSize(12).TextColor(k.textFaint)
		return
	case fileTreeFailed:
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, directory.err).FontSize(12).TextColor(k.attention).LineHeight(1.35)
			if iconButton(c, k, "rotate-ccw", tr("Retry"), 25, 14).Tooltip(tr("Retry")).Clicked() {
				a.loadFileTreeDirectory(directory.requested, true)
			}
		})
		return
	}
	if len(directory.entries) == 0 {
		ui.Text(c, tr("This folder is empty")).FontSize(12).TextColor(k.textFaint)
		return
	}
	for _, entry := range directory.entries {
		childKey := ""
		accessibleLabel := fmt.Sprintf(tr("File: %s"), entry.Name)
		if entry.Kind == filetree.EntryDirectory {
			accessibleLabel = fmt.Sprintf(tr("Folder: %s"), entry.Name)
			if child, err := fileTreeChildLocation(effectiveFileTreeLocation(directory), entry.Name); err == nil {
				childKey = fileTreeLocationKey(child)
			}
		} else if entry.Kind == filetree.EntrySymlink {
			accessibleLabel = fmt.Sprintf(tr("Symbolic link: %s"), entry.Name)
		}
		row := ui.Row(c.Key("file tree row "+key+"/"+entry.Name)).Height(27).Padding(0, 7, 0, 4+float32(depth*15)).
			Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(accessibleLabel)
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(tr("Copy Path")).Chosen() {
				a.copyFileTreeEntry(c, directory, entry.Name)
			}
			m.Separator()
			if entry.Kind == filetree.EntryDirectory {
				action := "Expand"
				if directory.expanded[childKey] {
					action = "Collapse"
				}
				if m.Item(tr(action)).Disabled(childKey == "").Chosen() {
					a.toggleFileTreeDirectory(key, entry)
				}
			}
			if m.Item(tr("Refresh Folder")).Chosen() {
				a.refreshFileTreeEntry(directory, entry)
			}
		})
		if row.Hovered() {
			row.Background(k.hover)
		}
		row.Children(func() {
			if entry.Kind == filetree.EntryDirectory {
				glyph := "chevrons-right"
				if directory.expanded[childKey] {
					glyph = "chevron-down"
				}
				ui.Icon(c, icon(glyph)).Size(12, 12).TextColor(k.iconMuted)
				ui.Icon(c, icon("folder-open")).Size(14, 14).TextColor(k.textMuted)
			} else {
				ui.Box(c).Size(12, 12)
				glyph := "file-code"
				ui.Icon(c, icon(glyph)).Size(14, 14).TextColor(k.iconMuted)
			}
			ui.Text(c, entry.Name).FontSize(12.5).TextColor(k.text).SingleLine().Ellipsis("…").MinWidth(0)
		})
		if row.Clicked() {
			if entry.Kind == filetree.EntryDirectory {
				a.toggleFileTreeDirectory(key, entry)
			} else {
				a.copyFileTreeEntry(c, directory, entry.Name)
			}
		}
		if childKey != "" && directory.expanded[childKey] {
			a.drawFileTreeDirectory(c, k, childKey, depth+1)
		}
	}
}
