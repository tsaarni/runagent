// Reads process start time via sysctl on macOS, used to detect PID reuse.

//go:build darwin

package daemon

import "golang.org/x/sys/unix"

func ReadStartTime(pid int) (int64, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	if int(kp.Proc.P_pid) != pid {
		return 0, unix.ESRCH
	}
	t := kp.Proc.P_starttime
	return int64(t.Sec)*1_000_000 + int64(t.Usec), nil
}
