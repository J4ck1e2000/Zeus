package zeus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The host answer is stubbed for the whole run: the goroutine that asks
// the machine about itself can outlive any one test, so its tools are
// kept out of the tests from the start.
func TestMain(m *testing.M) {
	hostInfoFn = func() HostInfo { return HostInfo{Name: "test"} }
	os.Exit(m.Run())
}

// startServer serves in this process on a data directory of its own, and
// returns a client of it. tune, when given, moves the timers before the
// server starts, so a test can watch them fire.
func startServer(t *testing.T, tune func()) *Client {
	t.Helper()
	t.Setenv("ZEUS_DIR", t.TempDir())
	oldIdle, oldTick, oldReap := idleTimeout, idleTick, reapDelay
	if tune != nil {
		tune()
	}
	serving := make(chan error, 1)
	var c *Client
	go func() { serving <- Serve() }()
	// The globals go back only once the server has stopped reading them,
	// so one test's server cannot race the next test's cleanup.
	t.Cleanup(func() {
		c.Shutdown()
		c.Close()
		select {
		case err := <-serving:
			if err != nil {
				t.Errorf("server: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("the server did not stop")
		}
		idleTimeout, idleTick, reapDelay = oldIdle, oldTick, oldReap
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(SocketPath()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the server did not listen")
		}
		time.Sleep(5 * time.Millisecond)
	}
	var err error
	if c, err = Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// The first request makes the connection count as one of the
	// server's, so a slow test does not look like an idle server.
	if _, err := c.Hello(); err != nil {
		t.Fatalf("hello: %v", err)
	}
	return c
}

// dial opens a raw connection, as a program without a token would.
func dial(t *testing.T) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("unix", SocketPath())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, bufio.NewReader(conn)
}

// readLine reads one answer, for the tests that speak the protocol raw.
func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return strings.TrimSpace(line)
}

func TestHello(t *testing.T) {
	c := startServer(t, nil)
	h, err := c.Hello()
	if err != nil {
		t.Fatalf("hello: %v", err)
	}
	if h.Version != ProtocolVersion {
		t.Errorf("hello says protocol v%d, want v%d", h.Version, ProtocolVersion)
	}
	if h.Host.Name != "test" {
		t.Errorf("hello names %q", h.Host.Name)
	}
}

func TestControlWithoutToken(t *testing.T) {
	startServer(t, nil)
	conn, r := dial(t)
	b, _ := json.Marshal(Request{Op: "list"})
	conn.Write(append(b, '\n'))
	if line := readLine(t, r); !strings.Contains(line, "unauthorized") {
		t.Errorf("a connection without a token got %q", line)
	}
}

func TestAttachWithoutToken(t *testing.T) {
	startServer(t, nil)
	conn, r := dial(t)
	b, _ := json.Marshal(Attach{Op: "attach", SID: "anything"})
	conn.Write(append(b, '\n'))
	if line := readLine(t, r); !strings.Contains(line, "unauthorized") {
		t.Errorf("an attach without a token got %q", line)
	}
}

func TestGarbageLine(t *testing.T) {
	c := startServer(t, nil)
	conn, r := dial(t)
	// A line the server cannot read ends the connection with an answer,
	// not silence; the client's own connection goes on working.
	conn.Write([]byte("not json\n"))
	if line := readLine(t, r); !strings.Contains(line, "bad request") {
		t.Errorf("a bad line got %q", line)
	}
	if _, err := r.Peek(1); err == nil {
		t.Error("the server kept the connection after a bad line")
	}
	if _, err := c.List(); err != nil {
		t.Errorf("list: %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the session runs a unix shell")
	}
	c := startServer(t, nil)
	info, err := c.Create(CreateOptions{
		Command: []string{"/bin/sh", "-c", "printf hello; sleep 30"}, Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if info.ID == "" {
		t.Fatal("the session has no id")
	}

	// What the session printed comes out of its stream.
	stream := c.Stream(info.ID, 80, 24)
	var got bytes.Buffer
	read := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stream.Read(buf)
			got.Write(buf[:n])
			if err != nil || bytes.Contains(got.Bytes(), []byte("hello")) {
				close(read)
				return
			}
		}
	}()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatalf("the session printed %q, with no hello in it", got.String())
	}

	list, err := c.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != info.ID {
		t.Fatalf("the server lists %v, want the one session %q", list, info.ID)
	}
	if err := c.Kill(info.ID); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if list, err = c.List(); err != nil {
		t.Fatalf("list: %v", err)
	} else if len(list) != 0 {
		t.Errorf("the server keeps %v after the kill", list)
	}
	stream.Close()
}

func TestExitedSessionReapedAndServerIdles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the session runs a unix shell")
	}
	c := startServer(t, func() {
		idleTimeout, idleTick, reapDelay = time.Second, 20*time.Millisecond, 50*time.Millisecond
	})
	if _, err := c.Create(CreateOptions{Command: []string{"true"}}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A session that ended by itself is dropped once its grace is over;
	// it used to keep the server alive forever.
	deadline := time.Now().Add(5 * time.Second)
	for {
		list, err := c.List()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the exited session was never reaped")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// With the session reaped and this client gone, nothing is left for
	// the server to keep: it ends itself and takes its socket with it.
	c.Close()
	deadline = time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(SocketPath()); os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the server never went idle")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestLockApp(t *testing.T) {
	t.Setenv("ZEUS_DIR", t.TempDir())
	release, err := LockApp()
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if _, err := LockApp(); err == nil {
		t.Error("a second app took the lock")
	}
	if err := release.Close(); err != nil {
		t.Errorf("unlock: %v", err)
	}
	if release, err = LockApp(); err != nil {
		t.Fatalf("lock again: %v", err)
	}
	release.Close()
}
