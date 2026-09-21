//go:build !windows

package board

import "syscall"

// detachAttr puts the board's server process in a session of its own, so
// closing the terminal that launched it does not take the board down
// with it.
func detachAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
