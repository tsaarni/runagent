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

func (sc *StatsCollector) Collect() (runagent.Stats, error) {
	return nil, errNotSupported
}
