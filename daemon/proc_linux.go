// Collects per-process resource stats (CPU, memory, I/O, FDs) from /proc on Linux.

//go:build linux

package daemon

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/tsaarni/runagent"
)

type StatsCollector struct {
	pid       int
	prevTicks uint64
	prevWhen  time.Time
}

func NewStatsCollector(pid int) *StatsCollector {
	return &StatsCollector{pid: pid}
}

func (sc *StatsCollector) Collect(mode StatsMode) (runagent.Stats, error) {
	var stats runagent.Stats

	fields, err := readStatFields(sc.pid)
	if err != nil {
		return nil, err
	}
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	threads, _ := strconv.ParseUint(fields[17], 10, 64)

	// CPU% calculation (always update ticks for accurate measurement)
	ticks := utime + stime
	now := time.Now()
	var cpuPct float64
	if sc.prevWhen.IsZero() {
		sc.prevTicks = ticks
		sc.prevWhen = now
	} else {
		dt := now.Sub(sc.prevWhen).Seconds()
		if dt > 0 {
			cpuPct = float64(ticks-sc.prevTicks) / (dt * float64(clkTck)) * 100
		}
		sc.prevTicks = ticks
		sc.prevWhen = now
	}

	if mode == Snapshot {
		stats = append(stats,
			runagent.Stat{Label: "CPU (3s avg)", Value: fmt.Sprintf("%.1f%%", cpuPct)},
			runagent.Stat{Label: "CPU time", Value: fmt.Sprintf("%.1fs user, %.1fs system", float64(utime)/float64(clkTck), float64(stime)/float64(clkTck))},
		)
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps_rollup", sc.pid))
	if err == nil {
		m := parseKeyValue(string(data))
		stats = append(stats,
			runagent.Stat{Label: "RSS", Value: humanize.IBytes(m["Rss"] * 1024)},
			runagent.Stat{Label: "PSS", Value: humanize.IBytes(m["Pss"] * 1024)},
		)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	data, err = os.ReadFile(fmt.Sprintf("/proc/%d/status", sc.pid))
	if err == nil {
		m := parseKeyValue(string(data))
		if mode == Snapshot {
			stats = append(stats, runagent.Stat{Label: "Peak RSS", Value: humanize.IBytes(m["VmHWM"] * 1024)})
		}
		stats = append(stats, runagent.Stat{Label: "Threads", Value: fmt.Sprintf("%d", threads)})

		// Count child processes
		if childData, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", sc.pid, sc.pid)); err == nil {
			n := len(strings.Fields(string(childData)))
			stats = append(stats, runagent.Stat{Label: "Child procs", Value: fmt.Sprintf("%d", n)})
		}

		entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", sc.pid))
		if err == nil {
			stats = append(stats, runagent.Stat{Label: "Open FDs", Value: fmt.Sprintf("%d", len(entries))})
		}

	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	data, err = os.ReadFile(fmt.Sprintf("/proc/%d/io", sc.pid))
	if err == nil {
		m := parseKeyValue(string(data))
		stats = append(stats, runagent.Stat{Label: "Disk I/O", Value: fmt.Sprintf("%s read, %s written",
			humanize.IBytes(m["read_bytes"]), humanize.IBytes(m["write_bytes"]))})
	}

	return stats, nil
}

func ReadStartTime(pid int) (int64, error) {
	fields, err := readStatFields(pid)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(fields[19], 10, 64)
}

func readStatFields(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return nil, fmt.Errorf("proc: invalid stat format for pid %d", pid)
	}
	return strings.Fields(s[idx+2:]), nil
}

// clkTck is the USER_HZ value used by the kernel when reporting CPU times in
// /proc/[pid]/stat. This is a stable Linux ABI constant (always 100) regardless
// of the kernel's internal CONFIG_HZ setting.
const clkTck = 100

func parseKeyValue(data string) map[string]uint64 {
	m := make(map[string]uint64)
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := strings.TrimSpace(parts[1])
		val = strings.TrimSuffix(val, " kB")
		if n, err := strconv.ParseUint(val, 10, 64); err == nil {
			m[key] = n
		}
	}
	return m
}
