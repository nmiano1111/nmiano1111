package sqlite

import (
	"os"
	"syscall"
)

// pidAlive reports whether a process with the given PID is currently running.
// It uses kill(pid, 0) which returns nil if the process exists and we have
// permission to signal it, EPERM if it exists but we lack permission, or
// ESRCH if it does not exist.
func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
