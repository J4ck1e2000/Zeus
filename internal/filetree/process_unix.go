//go:build unix

package filetree

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup runs the command in its own process group and kills the
// whole group when the command's context is done, so an ssh grandchild
// holding the inherited stderr cannot outlive its cancelled parent.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			// A missing group means the command already exited; report it the
			// way the default cancel would so Wait keeps the real exit status.
			if err == syscall.ESRCH {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}
