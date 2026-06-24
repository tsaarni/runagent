// Provides Linux-specific child process attributes including Pdeathsig for orphan cleanup.

//go:build linux

package daemon

import "syscall"

// sysProcAttr returns process attributes for spawned children.
// Pdeathsig ensures the kernel sends SIGKILL to children if the daemon dies
// (including hard-kill with SIGKILL). This has no equivalent on other platforms.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
