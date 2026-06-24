// Provides a no-op stats collector stub for non-Linux platforms.

//go:build !linux

package daemon

import (
	"errors"

	"github.com/tsaarni/runagent"
)

var errNotSupported = errors.New("process stats not supported on this platform")

type StatsCollector struct{}

func NewStatsCollector(pid int) *StatsCollector {
	return &StatsCollector{}
}

func (sc *StatsCollector) Collect(mode StatsMode) (runagent.Stats, error) {
	return nil, errNotSupported
}

func ReadStartTime(pid int) (int64, error) {
	return 0, errNotSupported
}
