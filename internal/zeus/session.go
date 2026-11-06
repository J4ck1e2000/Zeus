//go:build darwin || linux || windows

package zeus

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/plugins/terminal"

	"zeus/internal/shellintegration"
)

// scrollback is about how many bytes of output a session keeps above its
// screen, for the windows that attach to it.
const scrollback = 8 << 20

// session is a pseudo-terminal of the server and its shell.
type session struct {
	id      string
	shell   string
	created time.Time
	p       *pty

	mu sync.Mutex
	// vt is the session's screen, kept by the terminal emulator of the
	// app's terminals without a view: a window attaching gets a snapshot
	// of it at its size, as the session shows it now.
	vt             *terminal.Terminal
	vtIn           *io.PipeWriter
	shellDir       string
	shellDirPID    int
	integrationDir string
	output         uint64
	bells          atomic.Int64
	clients        map[*attached]struct{}
	lastOutput     time.Time
	lastInput      time.Time
	resync         *time.Timer
	exited         bool
	code           int
	cols, rows     int
	done           chan struct{}
}

// attached is a connection attached to a session, whose output a
// goroutine writes so that a slow client never holds up the others.
type attached struct {
	conn net.Conn
	out  chan []byte
	once sync.Once
}

func (a *attached) send(p []byte) bool {
	select {
	case a.out <- p:
		return true
	default:
		return false // too far behind: drop it, it attaches again
	}
}

func (a *attached) finish() { a.once.Do(func() { close(a.out) }) }

func (a *attached) writer() {
	for p := range a.out {
		if _, err := a.conn.Write(p); err != nil {
			break
		}
	}
	a.conn.Close()
	for range a.out {
	}
}

func newSession(id string, o CreateOptions) (*session, error) {
	path, argv, name, err := shellCommand(o.Command)
	if err != nil {
		return nil, err
	}
	cols, rows := o.Cols, o.Rows
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}
	dir := o.Dir
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	env := sessionEnv(id, o.Env)
	integrationDir := ""
	if shell := shellintegration.Detect(path); shell != shellintegration.Unknown {
		candidateDir := filepath.Join(Dir(), "shell-integration", id)
		launch, prepareErr := shellintegration.PrepareLocal(shell, shellintegration.LocalOptions{
			Executable: path, Args: argv, IntegrationDir: candidateDir, Environment: env,
		})
		if prepareErr != nil {
			log.Printf("zeus: preparing %v shell integration: %v", shell, prepareErr)
		} else {
			changed := launch.Path != path || !slices.Equal(launch.Args, argv) || len(launch.Env) > 0 || len(launch.Files) > 0
			if changed && len(launch.Files) > 0 {
				if writeErr := writeShellIntegrationFiles(candidateDir, launch.Files); writeErr != nil {
					log.Printf("zeus: writing %v shell integration: %v", shell, writeErr)
					_ = os.RemoveAll(candidateDir)
				} else {
					integrationDir = candidateDir
				}
			}
			if changed && (len(launch.Files) == 0 || integrationDir != "") {
				path, argv = launch.Path, launch.Args
				env = mergeEnvironment(env, launch.Env)
			}
		}
	}
	p, err := startPTY(path, argv, dir, env, cols, rows)
	if err != nil {
		if integrationDir != "" {
			_ = os.RemoveAll(integrationDir)
		}
		return nil, err
	}
	s := &session{
		id: id, shell: name, created: time.Now(), p: p,
		clients: map[*attached]struct{}{},
		cols:    cols, rows: rows, done: make(chan struct{}), integrationDir: integrationDir,
	}
	// The emulator's Conn reads nothing, and what it would answer programs
	// is dropped: the windows' terminals answer them.
	pr, pw := io.Pipe()
	vt, err := terminal.New(terminal.Options{
		Conn: discard{pr}, Scrollback: scrollback,
		OnBell: func() { s.bells.Add(1) },
		OnPwd: func(string) {
			fg := s.p.foreground()
			if fg <= 0 {
				fg = s.p.pid()
			}
			p := inspect(fg)
			s.shellDir, s.shellDirPID = shellDirForProcess(p.programName(), fg, s.p.pid(), s.shell, s.vt.Dir())
		},
	})
	if err != nil {
		p.hangup()
		if integrationDir != "" {
			_ = os.RemoveAll(integrationDir)
		}
		return nil, err
	}
	vt.Resize(cols, rows)
	s.vt, s.vtIn = vt, pw
	read := make(chan struct{})
	go func() {
		s.read()
		close(read)
	}()
	go func() {
		code := p.wait()
		// What it printed last is still to read; then hang up whatever
		// else holds the terminal, as terminal apps do.
		select {
		case <-read:
		case <-time.After(150 * time.Millisecond):
			p.close()
			<-read
		}
		s.mu.Lock()
		s.exited, s.code = true, code
		for a := range s.clients {
			a.finish()
		}
		clear(s.clients)
		s.mu.Unlock()
		s.vtIn.Close()
		s.vt.Close()
		if s.integrationDir != "" {
			_ = os.RemoveAll(s.integrationDir)
		}
		close(s.done)
	}()
	return s, nil
}

// sessionEnv returns the environment of a session's shell: the server's,
// without what other terminals set, and what terminal apps set.
func sessionEnv(id string, extra []string) []string {
	var env []string
	hasLang := false
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case k == "TERM", k == "COLORTERM", k == "TERM_PROGRAM", k == "TERM_PROGRAM_VERSION",
			k == "TERM_SESSION_ID", k == "TMUX", k == "TMUX_PANE", k == "STY", k == "WINDOWID",
			k == "COLUMNS", k == "LINES", k == "SHLVL", k == "OLDPWD", k == "PWD", k == "_",
			strings.HasPrefix(k, "ITERM_"), strings.HasPrefix(k, "GHOSTTY_"), strings.HasPrefix(k, "KITTY_"),
			strings.HasPrefix(k, "VSCODE_"), strings.HasPrefix(k, "WEZTERM_"), strings.HasPrefix(k, "ALACRITTY_"),
			strings.HasPrefix(k, "ZEUS_"), strings.HasPrefix(k, "MYGO_"):
			continue
		case k == "LANG" || k == "LC_ALL" || k == "LC_CTYPE":
			hasLang = true
		}
		env = append(env, kv)
	}
	env = append(env,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"TERM_PROGRAM=Zeus",
		"TERM_PROGRAM_VERSION=0.1.0",
		"ZEUS_SESSION="+id,
	)
	if !hasLang {
		env = append(env, "LANG=en_US.UTF-8")
	}
	return mergeEnvironment(env, extra)
}

func mergeEnvironment(base, overrides []string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	indices := make(map[string]int, len(base)+len(overrides))
	key := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	for _, item := range append(append([]string(nil), base...), overrides...) {
		name, _, ok := strings.Cut(item, "=")
		if !ok || name == "" {
			result = append(result, item)
			continue
		}
		indexKey := key(name)
		if index, exists := indices[indexKey]; exists {
			result[index] = item
		} else {
			indices[indexKey] = len(result)
			result = append(result, item)
		}
	}
	return result
}

func writeShellIntegrationFiles(dir string, files []shellintegration.File) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if filepath.IsAbs(file.Name) {
			return fmt.Errorf("shell integration file %q must be relative", file.Name)
		}
		name := filepath.Clean(file.Name)
		if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("shell integration file %q escapes its directory", file.Name)
		}
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		mode := file.Mode
		if mode == 0 {
			mode = 0o600
		}
		if err := os.WriteFile(path, file.Content, mode); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) read() {
	buf := make([]byte, 64<<10)
	for {
		n, err := s.p.read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.vt.Feed(data)
			s.output += uint64(n)
			s.lastOutput = time.Now()
			for a := range s.clients {
				if !a.send(data) {
					a.finish()
					delete(s.clients, a)
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// attach attaches a connection: it gets a snapshot of the session's
// screen and scrollback at its size, then what the session prints, and
// what it sends is typed.
func (s *session) attach(conn net.Conn, cols, rows int) {
	a := &attached{conn: conn, out: make(chan []byte, 2048)}
	s.mu.Lock()
	if cols > 0 && rows > 0 {
		// The screen reflows to the window's size, which the program
		// learns next.
		s.vt.Resize(cols, rows)
	}
	snap := s.vt.Snapshot()
	if title := s.vt.Title(); title != "" {
		snap = append([]byte("\x1b]2;"+title+"\x07"), snap...)
	}
	a.out <- snap
	exited := s.exited
	if exited {
		a.finish()
	} else {
		s.clients[a] = struct{}{}
	}
	s.mu.Unlock()
	go a.writer()
	if exited {
		return
	}
	if cols > 0 && rows > 0 {
		s.resize(cols, rows)
	}
	buf := make([]byte, 32<<10)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			s.input(buf[:n])
		}
		if err != nil {
			break
		}
	}
	s.mu.Lock()
	if _, ok := s.clients[a]; ok {
		delete(s.clients, a)
		a.finish()
	}
	s.mu.Unlock()
}

func (s *session) input(p []byte) {
	s.mu.Lock()
	s.lastInput = time.Now()
	exited := s.exited
	s.mu.Unlock()
	if !exited {
		s.p.write(p)
	}
}

// resize sets the size of the terminal and reports whether it changed.
func (s *session) resize(cols, rows int) bool {
	s.mu.Lock()
	changed := cols != s.cols || rows != s.rows
	s.cols, s.rows = cols, rows
	exited := s.exited
	if changed {
		s.vt.Resize(cols, rows)
		// Once the program drew its screen at the new size, the windows
		// get it again.
		if s.resync == nil {
			s.resync = time.AfterFunc(resyncDelay, s.resyncScreen)
		} else {
			s.resync.Reset(resyncDelay)
		}
	}
	s.mu.Unlock()
	if changed && !exited {
		s.p.resize(cols, rows)
	}
	return changed
}

// resyncDelay is how long after the last resize the screen of a
// full-screen program is sent again.
const resyncDelay = 150 * time.Millisecond

// resyncScreen sends the windows the screen of a full-screen program as
// the session has it. A window resizes its terminal as it draws, and the
// session as the request reaches it, while the program's output is on its
// way: what the program drew for one size may land in a screen of
// another, which full-screen programs, redrawing only what changed, do
// not mend. Shells' scrollback reflows alike in both and needs nothing.
func (s *session) resyncScreen() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited || len(s.clients) == 0 {
		return
	}
	snap := s.vt.Snapshot()
	if !bytes.Contains(snap, []byte("\x1b[?1049h")) {
		return
	}
	// Clear the alternate screen the window shows, then draw it anew.
	msg := append([]byte("\x1b[?1049h\x1b[H\x1b[2J"), snap...)
	for a := range s.clients {
		if !a.send(msg) {
			a.finish()
			delete(s.clients, a)
		}
	}
}

// kill hangs up the session and waits a little for it to end.
func (s *session) kill() {
	s.mu.Lock()
	exited := s.exited
	s.mu.Unlock()
	if !exited {
		s.p.hangup()
	}
	select {
	case <-s.done:
	case <-time.After(time.Second):
		s.p.kill()
	}
}

// info describes the session and what runs in its foreground.
func (s *session) info() SessionInfo {
	s.mu.Lock()
	in := SessionInfo{
		ID: s.id, Shell: s.shell, Created: s.created,
		Title: s.vt.Title(), LastOutput: s.lastOutput, LastInput: s.lastInput,
		Output: s.output, Bells: int(s.bells.Load()), Exited: s.exited, ExitCode: s.code,
		Attached: len(s.clients), Cols: s.cols, Rows: s.rows,
	}
	shellDir, shellDirPID := s.shellDir, s.shellDirPID
	s.mu.Unlock()
	in.PID = s.p.pid()
	if in.Exited {
		return in
	}
	fg := s.p.foreground()
	if fg <= 0 {
		fg = in.PID
	}
	in.Idle = fg == in.PID
	p := inspect(fg)
	in.Program, in.Args, in.Dir = p.programName(), p.args, p.dir
	if in.Program == "" {
		in.Program = s.shell
	}
	if fg == shellDirPID && (strings.EqualFold(in.Program, "ssh") || strings.EqualFold(in.Program, s.shell)) {
		in.ShellDir = shellDir
	}
	if in.Dir == "" && !in.Idle {
		in.Dir = inspect(in.PID).dir
	}
	return in
}

func shellDirForProcess(program string, pid, sessionPID int, shell, dir string) (string, int) {
	if pid <= 0 || !path.IsAbs(dir) {
		return "", 0
	}
	if strings.EqualFold(program, "ssh") {
		return dir, pid
	}
	if pid == sessionPID && strings.EqualFold(program, shell) {
		return dir, pid
	}
	return "", 0
}

// discard is the Conn of a session's emulator: it reads from a pipe that
// closes with the session, and drops what it writes.
type discard struct{ r *io.PipeReader }

func (d discard) Read(p []byte) (int, error)  { return d.r.Read(p) }
func (d discard) Write(p []byte) (int, error) { return len(p), nil }
func (d discard) Close() error                { return d.r.Close() }
