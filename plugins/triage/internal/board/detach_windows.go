//go:build windows

package board

import "syscall"

// detachAttr is the Windows counterpart of the Unix Setsid: the board's
// server process is given its own process group and no console, so a
// Ctrl-C in the launching console does not reach it and it survives that
// console closing.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
}
