//go:build darwin || linux || windows

package zeus

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// appLockCloser releases the app's single-instance lock.
type appLockCloser struct{ f *os.File }

// Close releases the lock and closes its file.
func (c appLockCloser) Close() error {
	unlockFile(c.f)
	return c.f.Close()
}

// LockApp takes the app's single-instance lock, app.lock in Dir(). It
// returns a closer that releases it, or an error when another app window
// runs. The lock is tried for a moment, so an app restarting over its old
// instance, which may still hold the lock as it goes, does not fail at
// once.
func LockApp() (io.Closer, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "app.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if lockFile(f) == nil {
			return appLockCloser{f: f}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("zeus: another app window already runs")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
