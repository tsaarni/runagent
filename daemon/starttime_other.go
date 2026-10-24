//go:build !linux && !darwin

package daemon

func ReadStartTime(pid int) (int64, error) {
	return 0, errNotSupported
}
