// Collects per-process resource stats (CPU, memory, I/O, FDs) from /proc on Linux.
// Network I/O is not included: Linux does not track network stats per process,
// only per network namespace (/proc/<pid>/net/dev).

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

func (sc *StatsCollector) Collect() (runagent.Stats, error) {
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

	stats = append(stats,
		runagent.Stat{Label: "CPU (3s avg)", Value: fmt.Sprintf("%.1f%%", cpuPct)},
		runagent.Stat{Label: "CPU time", Value: fmt.Sprintf("%.1fs user, %.1fs system", float64(utime)/float64(clkTck), float64(stime)/float64(clkTck))},
	)

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
		stats = append(stats, runagent.Stat{Label: "Peak RSS", Value: humanize.IBytes(m["VmHWM"] * 1024)})
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

	if ports := readListenPorts(sc.pid); len(ports) > 0 {
		stats = append(stats, runagent.Stat{Label: "Listen", Value: strings.Join(ports, ", ")})
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

// readListenPorts returns listening TCP ports owned by the given process.
// It reads /proc/<pid>/net/tcp{,6} and filters by socket inodes found in /proc/<pid>/fd.
func readListenPorts(pid int) []string {
	ownedInodes := readSocketInodes(pid)
	if len(ownedInodes) == 0 {
		return nil
	}

	var ports []string
	seen := make(map[string]bool)

	for _, proto := range []string{"tcp", "tcp6"} {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/net/%s", pid, proto))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			// State 0A = LISTEN
			if fields[3] != "0A" {
				continue
			}
			inode := fields[9]
			if !ownedInodes[inode] {
				continue
			}
			addr, port := parseHexAddrPort(fields[1], proto == "tcp6")
			s := formatListenAddr(addr, port)
			if !seen[s] {
				seen[s] = true
				ports = append(ports, s)
			}
		}
	}
	return ports
}

// readSocketInodes returns the set of socket inode numbers owned by the process.
func readSocketInodes(pid int) map[string]bool {
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		return nil
	}
	inodes := make(map[string]bool)
	for _, e := range entries {
		link, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, e.Name()))
		if err != nil {
			continue
		}
		// Format: socket:[12345]
		if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
			inodes[link[8:len(link)-1]] = true
		}
	}
	return inodes
}

// parseHexAddrPort extracts IP and port from the hex-encoded local_address field.
func parseHexAddrPort(s string, isV6 bool) (string, uint16) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", 0
	}
	port, _ := strconv.ParseUint(parts[1], 16, 16)

	hexAddr := parts[0]
	if !isV6 {
		// IPv4: 4 bytes in little-endian hex
		if len(hexAddr) == 8 {
			a, _ := strconv.ParseUint(hexAddr[6:8], 16, 8)
			b, _ := strconv.ParseUint(hexAddr[4:6], 16, 8)
			c, _ := strconv.ParseUint(hexAddr[2:4], 16, 8)
			d, _ := strconv.ParseUint(hexAddr[0:2], 16, 8)
			return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d), uint16(port)
		}
		return "", uint16(port)
	}

	// IPv6: 32 hex chars, groups of 8 chars each in little-endian 32-bit words
	if len(hexAddr) == 32 {
		// Check for IPv4-mapped IPv6 (::ffff:x.x.x.x)
		if hexAddr[:24] == "0000000000000000FFFF0000" {
			v4 := hexAddr[24:]
			a, _ := strconv.ParseUint(v4[6:8], 16, 8)
			b, _ := strconv.ParseUint(v4[4:6], 16, 8)
			c, _ := strconv.ParseUint(v4[2:4], 16, 8)
			d, _ := strconv.ParseUint(v4[0:2], 16, 8)
			return fmt.Sprintf("%d.%d.%d.%d", a, b, c, d), uint16(port)
		}
		// All zeros = ::
		if hexAddr == "00000000000000000000000000000000" {
			return "::", uint16(port)
		}
		return "[::]", uint16(port)
	}
	return "", uint16(port)
}

func formatListenAddr(addr string, port uint16) string {
	if addr == "0.0.0.0" || addr == "::" {
		return fmt.Sprintf(":%d", port)
	}
	if strings.Contains(addr, ":") {
		return fmt.Sprintf("[%s]:%d", addr, port)
	}
	return fmt.Sprintf("%s:%d", addr, port)
}
