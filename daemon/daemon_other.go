// Provides the non-Linux fallback for child process attributes (setpgid only).

//go:build !linux

package daemon

import "syscall"

func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
