//go:build !darwin && !linux && !windows

package zeus

import (
	"errors"
	"io"
)

var errUnsupported = errors.New("zeus: the session server runs on macOS, Linux and Windows")

// Serve runs the server; not on this platform.
func Serve() error { return errUnsupported }

// Spawn starts a server; not on this platform.
func Spawn() error { return errUnsupported }

// LockApp takes the app's lock; not on this platform.
func LockApp() (io.Closer, error) { return nil, errUnsupported }

func processAlive(int) bool { return false }
