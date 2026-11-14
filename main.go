// Zeus is a terminal of tabs and split panes, after Superlogical's Rex,
// in MyGo's native UI. Its shells run in a session server of their own,
// so that they outlive the app: quit it, open it again, and every tab and
// pane is back as it was, its programs still running.
//
//	go tool mygo dev    the app, rebuilt as the code changes
//	go run . -server    the session server alone, in the foreground
package main

import (
	"encoding/json"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"zeus/internal/zeus"
)

func main() {
	// The state of the sessions lives in the app's data directory, which
	// the server it starts takes from ZEUS_DIR: "Zeus", or "Zeus Dev"
	// under mygo dev, which keeps the installed app's sessions apart.
	if os.Getenv("ZEUS_DIR") == "" {
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			os.Setenv("ZEUS_DIR", dir)
		}
	}
	// -server runs the session server; other arguments are AppKit's, as
	// -NSWindowResizeTime, which it reads itself.
	if len(os.Args) > 1 && os.Args[1] == "-server" {
		log.SetPrefix("[server] ")
		if err := zeus.Serve(); err != nil {
			log.Fatal(err)
		}
		return
	}
	// One window of Zeus at a time: two would attach to every session
	// and overwrite each other's layout.
	if _, err := zeus.LockApp(); err != nil {
		log.Fatalf("Zeus is already running: %v", err)
	}
	registerFonts()
	loadSettings()
	a := &App{}
	buildMenu = a.menu
	allCommands = commands
	mygo.App.WhenReady(func() {
		// The menus built at startup followed the environment's stand-in
		// for the system's language; rebuild them with the real one.
		resolveSystemLanguage()
		a.rebuildMenu()
	})
	mygo.App.SetMenu(a.menu())
	mygo.App.WhenReady(a.open)
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows && a.win == nil {
			a.open()
		}
	})
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.saveNow()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// open opens the window, with the tabs the server kept.
func (a *App) open() {
	client, err := zeus.Connect()
	if err != nil {
		log.Print(err)
		mygo.Dialog.Error(tr("Zeus could not start its session server"), err.Error())
		mygo.App.Quit()
		return
	}
	a.client = client
	a.hello, err = client.Hello()
	if err == nil && mygo.IsDev() && staleServer(a.hello) {
		// A server a build before this one started runs that build's
		// code: start one of this build's, ending its sessions.
		if client, err = client.Restart(a.hello.PID); err == nil {
			a.client = client
			a.hello, err = client.Hello()
		}
		if err != nil {
			log.Print(err)
			mygo.Dialog.Error(tr("Zeus could not restart its session server"), err.Error())
			mygo.App.Quit()
			return
		}
	}
	if err != nil {
		// The hello fails when the server speaks another protocol's
		// version, among its other ways of going: none of them opens a
		// window.
		log.Print(err)
		mygo.Dialog.Error(tr("Zeus could not start its session server"), err.Error())
		mygo.App.Quit()
		return
	}
	a.reset()
	opts := mygo.WindowOptions{
		Title:                "Zeus",
		Width:                1000,
		Height:               620,
		MinWidth:             560,
		MinHeight:            340,
		StateKey:             "main",
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 16, Y: 15},
		BackgroundColor:      "light-dark(#ccd9ec, #262033)",
		Content:              ui.View(a.view),
	}
	if ownControls {
		// The title bar draws the window's controls (windowControls).
		opts.TitleBarStyle, opts.Frameless = mygo.TitleBarDefault, true
	}
	win := mygo.NewWindow(opts)
	a.win = win
	if !a.restore() {
		a.newTab(a.newTabDir())
	}
	a.changed()
	go a.poll(win, client)
	a.debugHook()
	win.OnClosed(func() {
		a.saveNow()
		if a.settingsWin != nil {
			a.settingsWin.Close()
		}
		a.quitting = true
		for _, t := range a.tabs {
			for _, p := range t.panes() {
				p.closed = true
				if p.term != nil {
					p.term.Close() // detaches: the session goes on
				}
			}
		}
		a.client.Close()
		a.win = nil
	})
}

// saveNow sends the layout to the server and waits for it to be kept.
func (a *App) saveNow() {
	if a.client == nil || len(a.tabs) == 0 && !a.quitting {
		return
	}
	b, err := json.Marshal(a.snapshot())
	if err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		a.client.SetLayout(b)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// reset forgets the state of a window that closed, keeping the server's
// connection, for the next window.
func (a *App) reset() {
	a.fileTree.reset()
	a.tabs, a.active, a.focusReq = nil, 0, nil
	a.paletteOpen, a.hostOpen, a.moreTabsOpen, a.renaming = false, false, false, nil
	a.recording, a.recordingErr, a.find = nil, "", findState{}
	a.saveDue, a.quitting, a.lastSnapshot, a.title = false, false, "", ""
}

// staleServer reports whether the server runs another build of the app
// than this one, as after mygo dev rebuilt it.
func staleServer(h zeus.Hello) bool {
	exe, at := zeus.Executable()
	return h.Exe != exe || !h.ExeTime.Equal(at)
}
