//go:build darwin || linux || windows

package zeus

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Server owns the sessions.
type Server struct {
	mu        sync.Mutex
	sessions  map[string]*session
	order     []string
	layout    json.RawMessage
	started   time.Time
	host      HostInfo
	hostDone  chan struct{}
	ln        net.Listener
	exe       string
	exeTime   time.Time
	controls  int
	idleFrom  time.Time
	quit      chan struct{}
	quitOnce  sync.Once
	watchDone chan struct{}
	token     string
}

// idleTimeout is how long a server with no sessions and no app waits
// before it exits. A var, so tests can shorten it.
var idleTimeout = 10 * time.Minute

// idleTick is how often the server looks for work it can let go of. A
// var, so tests can shorten it.
var idleTick = 30 * time.Second

// hostInfoFn is what describes the machine; a var, so tests can answer
// without the tools the real one asks.
var hostInfoFn = hostInfo

// reapDelay is how long a session that ended by itself stays in the
// server, for a window to read what it printed last, before it is
// dropped. A var, so tests can shorten it.
var reapDelay = 5 * time.Minute

// Serve runs the server until it is shut down: it returns an error at
// once when another server already runs.
func Serve() error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		log.Printf("zeus: chmod %s: %v", dir, err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "server.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := lockFile(lock); err != nil {
		return errors.New("zeus: a server already runs")
	}
	defer lock.Close()
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	// The token is in place before the socket is: once a connection can
	// arrive, it can be told no.
	if err := os.WriteFile(filepath.Join(dir, "server.token"), []byte(token), 0o600); err != nil {
		return err
	}
	sock := SocketPath()
	os.Remove(sock)
	// The socket is the only gate of the server: what the listen creates
	// is kept to this user, whatever the umask the process inherited. It
	// stays set for the process's life, as restoring it would race the
	// goroutines that write files.
	tightenUmask()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		log.Printf("zeus: chmod %s: %v", sock, err)
	}
	s := &Server{
		sessions: map[string]*session{}, started: time.Now(), ln: ln,
		hostDone: make(chan struct{}), idleFrom: time.Now(), quit: make(chan struct{}),
		watchDone: make(chan struct{}),
		token:     token,
	}
	if b, err := os.ReadFile(filepath.Join(dir, "layout.json")); err == nil && json.Valid(b) {
		s.layout = b
	}
	s.exe, s.exeTime = Executable()
	go func() {
		s.mu.Lock()
		s.host = hostInfoFn()
		s.mu.Unlock()
		close(s.hostDone)
	}()
	go s.watchIdle()
	go func() {
		<-s.quit
		ln.Close()
	}()
	log.Printf("zeus: serving on %s (pid %d)", sock, os.Getpid())
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				s.mu.Lock()
				all := make([]*session, 0, len(s.sessions))
				for _, ss := range s.sessions {
					all = append(all, ss)
				}
				s.mu.Unlock()
				// A kill can wait a second for its session: end them
				// together, or shutdown outruns them.
				var wg sync.WaitGroup
				for _, ss := range all {
					wg.Add(1)
					go func() {
						defer wg.Done()
						ss.kill()
					}()
				}
				wg.Wait()
				os.Remove(sock)
				// The watcher stops last, and Serve leaves nothing
				// behind that reads the server.
				<-s.watchDone
				return nil
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) watchIdle() {
	defer close(s.watchDone)
	t := time.NewTicker(idleTick)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
		}
		s.mu.Lock()
		live, reaped := 0, false
		for id, ss := range s.sessions {
			ss.mu.Lock()
			exited, exitAt := ss.exited, ss.exitAt
			ss.mu.Unlock()
			if !exited {
				live++
				continue
			}
			// A session that ended by itself is dropped once nothing
			// would want it again: it kept the server alive forever.
			if time.Since(exitAt) > reapDelay {
				delete(s.sessions, id)
				s.order = slices.DeleteFunc(s.order, func(x string) bool { return x == id })
				reaped = true
			}
		}
		if reaped && live == 0 {
			// The last sessions are gone: the server is idle from now,
			// as it is when the kill op ends them.
			s.idleFrom = time.Now()
		}
		idle := live == 0 && s.controls == 0 && time.Since(s.idleFrom) > idleTimeout
		s.mu.Unlock()
		if idle {
			log.Print("zeus: idle, exiting")
			s.shutdown()
			return
		}
	}
}

func (s *Server) shutdown() { s.quitOnce.Do(func() { close(s.quit) }) }

func (s *Server) handle(conn net.Conn) {
	r := bufio.NewReaderSize(conn, 64<<10)
	// A connection that sends nothing must not park this goroutine
	// forever, and a line is at most a request.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	first, err := r.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})
	if len(first) > 1<<20 {
		conn.Close()
		return
	}
	var a Attach
	if json.Unmarshal(first, &a) == nil && a.Op == "attach" {
		if a.Token != s.token {
			conn.Write([]byte(`{"error":"unauthorized"}` + "\n"))
			conn.Close()
			return
		}
		s.mu.Lock()
		ss := s.sessions[a.SID]
		s.mu.Unlock()
		if ss == nil {
			conn.Write([]byte(`{"error":"no such session"}` + "\n"))
			conn.Close()
			return
		}
		conn.Write([]byte(`{"ok":true}` + "\n"))
		// What the reader buffered past the first line is input.
		if n := r.Buffered(); n > 0 {
			b, _ := r.Peek(n)
			ss.input(b)
		}
		ss.attach(conn, a.Cols, a.Rows)
		return
	}
	s.control(conn, r, first)
}

// control serves a control connection: a request per line.
func (s *Server) control(conn net.Conn, r *bufio.Reader, first []byte) {
	s.mu.Lock()
	s.controls++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.controls--
		s.idleFrom = time.Now()
		s.mu.Unlock()
		conn.Close()
	}()
	var wmu sync.Mutex
	reply := func(res Response) {
		b, _ := json.Marshal(res)
		wmu.Lock()
		conn.Write(append(b, '\n'))
		wmu.Unlock()
	}
	authed := false
	line := first
	for {
		if len(line) > 1<<20 {
			reply(Response{Error: "bad request"})
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			reply(Response{Error: "bad request"})
			return
		}
		if !authed {
			// The first line of a connection carries the token, and
			// nothing answers a connection that lacks it.
			if req.Token != s.token {
				reply(Response{ID: req.ID, Error: "unauthorized"})
				return
			}
			authed = true
		}
		// Kills wait for the session to end: answer them aside.
		if req.Op == "kill" {
			go func() { reply(s.request(req)) }()
		} else {
			reply(s.request(req))
		}
		var err error
		line, err = r.ReadBytes('\n')
		if err != nil {
			return
		}
	}
}

func (s *Server) request(req Request) Response {
	data, err := s.do(req)
	res := Response{ID: req.ID}
	if err != nil {
		res.Error = err.Error()
	} else if data != nil {
		res.Data, _ = json.Marshal(data)
	}
	return res
}

func (s *Server) session(id string) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ss := s.sessions[id]; ss != nil {
		return ss, nil
	}
	return nil, fmt.Errorf("no session %q", id)
}

func (s *Server) do(req Request) (any, error) {
	switch req.Op {
	case "hello":
		select {
		case <-s.hostDone:
		case <-time.After(3 * time.Second):
		}
		s.mu.Lock()
		host := s.host
		s.mu.Unlock()
		return Hello{Version: ProtocolVersion, PID: os.Getpid(), Started: s.started, Host: host, Exe: s.exe, ExeTime: s.exeTime}, nil
	case "list":
		s.mu.Lock()
		all := make([]*session, 0, len(s.order))
		for _, id := range s.order {
			all = append(all, s.sessions[id])
		}
		s.mu.Unlock()
		infos := make([]SessionInfo, len(all))
		for i, ss := range all {
			infos[i] = ss.info()
		}
		return infos, nil
	case "create":
		o := CreateOptions{}
		if req.Create != nil {
			o = *req.Create
		}
		id := newID()
		ss, err := newSession(id, o)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.sessions[id] = ss
		s.order = append(s.order, id)
		s.mu.Unlock()
		return ss.info(), nil
	case "kill":
		ss, err := s.session(req.SID)
		if err != nil {
			return nil, nil // already gone
		}
		ss.kill()
		s.mu.Lock()
		delete(s.sessions, req.SID)
		s.order = slices.DeleteFunc(s.order, func(id string) bool { return id == req.SID })
		if len(s.sessions) == 0 {
			s.idleFrom = time.Now()
		}
		s.mu.Unlock()
		return nil, nil
	case "resize":
		ss, err := s.session(req.SID)
		if err != nil {
			return nil, err
		}
		ss.resize(req.Cols, req.Rows)
		return nil, nil
	case "getLayout":
		s.mu.Lock()
		l := s.layout
		s.mu.Unlock()
		if l == nil {
			return nil, nil
		}
		return l, nil
	case "setLayout":
		if !json.Valid(req.Layout) {
			return nil, errors.New("layout is not JSON")
		}
		l := bytes.Clone(req.Layout)
		s.mu.Lock()
		s.layout = l
		s.mu.Unlock()
		path := filepath.Join(Dir(), "layout.json")
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, l, 0o600); err != nil {
			log.Printf("zeus: saving the layout: %v", err)
		} else if err := os.Rename(tmp, path); err != nil {
			log.Printf("zeus: saving the layout: %v", err)
		}
		return nil, nil
	case "shutdown":
		go func() {
			time.Sleep(50 * time.Millisecond)
			s.shutdown()
		}()
		return nil, nil
	}
	return nil, fmt.Errorf("unknown op %q", req.Op)
}

func newID() string {
	var b [6]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Spawn starts a server in the background, as a process of its own that
// outlives the app: the executable run with -server.
func Spawn() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(Dir(), "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd := exec.Command(exe, "-server")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, logf, logf
	cmd.SysProcAttr = detached()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
