//go:build windows

package filetree

import "os/exec"

// setProcessGroup does nothing on windows: the command keeps its WaitDelay
// and a stray grandchild dies when the inherited pipe it holds closes.
func setProcessGroup(cmd *exec.Cmd) {}
