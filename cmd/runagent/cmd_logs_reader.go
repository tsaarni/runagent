// Provides the log file reader with filtering, follow mode, and formatted output rendering.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tsaarni/runagent"
)

type logFilter struct {
	types     map[string]bool
	stream    string
	timeRange TimeRange
	limit     int
	last      int
}

type logRecord struct {
	Raw    json.RawMessage
	Header runagent.EventHeader
	Log    runagent.LogEvent
	Stop   runagent.StopEvent
	Start  runagent.StartEvent
	Stats  runagent.StatsEvent
}

func readLog(ctx context.Context, path string, filter logFilter, follow bool, jsonOutput bool, timeFormat string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var records []logRecord
	er := runagent.NewEventReader(f)
	for {
		h, raw, err := er.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		r := decodeRecord(h, raw)
		if matchesFilter(r, filter) {
			records = append(records, r)
		}
	}

	// Apply last/limit
	if filter.last > 0 && filter.last < len(records) {
		records = records[len(records)-filter.last:]
	} else if filter.limit > 0 && filter.limit < len(records) {
		records = records[:filter.limit]
	}

	p := &logPrinter{timeFormat: timeFormat}
	for _, r := range records {
		printRecord(p, r, jsonOutput, w)
	}

	if !follow {
		return nil
	}

	// Follow mode
	info, err := f.Stat()
	if err != nil {
		return err
	}
	lastSize := info.Size()

	count := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}

		info, err = os.Stat(path)
		if err != nil {
			return err
		}
		if info.Size() == lastSize {
			continue
		}
		lastSize = info.Size()

		for {
			h, raw, err := er.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			r := decodeRecord(h, raw)
			if !matchesFilter(r, filter) {
				continue
			}
			printRecord(p, r, jsonOutput, w)
			if r.Header.Type == "stop" {
				return nil
			}
			if filter.limit > 0 {
				count++
				if count >= filter.limit {
					return nil
				}
			}
		}
	}
}

func decodeRecord(h runagent.EventHeader, raw json.RawMessage) logRecord {
	r := logRecord{Raw: raw, Header: h}
	switch h.Type {
	case "start":
		_ = json.Unmarshal(raw, &r.Start)
	case "stop":
		_ = json.Unmarshal(raw, &r.Stop)
	case "log":
		_ = json.Unmarshal(raw, &r.Log)
	case "stats":
		_ = json.Unmarshal(raw, &r.Stats)
	}
	return r
}

func matchesFilter(r logRecord, f logFilter) bool {
	if !f.types[r.Header.Type] {
		return false
	}
	if f.stream != "" && r.Header.Type == "log" && r.Log.Stream != f.stream {
		return false
	}
	if f.timeRange.From != nil || f.timeRange.To != nil {
		ts := recordTime(r)
		if !ts.IsZero() {
			if f.timeRange.From != nil && ts.Before(*f.timeRange.From) {
				return false
			}
			if f.timeRange.To != nil && ts.After(*f.timeRange.To) {
				return false
			}
		}
	}
	return true
}

func recordTime(r logRecord) time.Time {
	s := r.Header.TS
	if s == "" {
		// start event uses its own TS field via EventHeader
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func printRecord(p *logPrinter, r logRecord, jsonOutput bool, w io.Writer) {
	if jsonOutput {
		_, _ = fmt.Fprintf(w, "%s\n", r.Raw)
		return
	}
	ts := recordTime(r)
	if !ts.IsZero() && p.timeFormat != "" {
		day := ts.Local().Format("2006-01-02")
		if p.lastDate != "" && day != p.lastDate {
			_, _ = fmt.Fprintf(w, "%s\n", dim("             ── "+day+" ──"))
		}
		p.lastDate = day
	}

	prefix := func(tsStr string, sep string) string {
		s := p.fmtTS(tsStr)
		if s == "" {
			return ""
		}
		return dimCyan(s) + " " + sep + " "
	}

	switch r.Header.Type {
	case "start":
		cmd := strings.Join(r.Start.Command, " ")
		_, _ = fmt.Fprintf(w, "%s%s\n", prefix(r.Start.TS, dim("│")), dimCyan("started "+cmd))
	case "stop":
		var msg string
		if r.Stop.Signal != 0 {
			msg = yellow(fmt.Sprintf("killed (signal %d)", r.Stop.Signal))
		} else if r.Stop.ExitCode != 0 {
			msg = red(fmt.Sprintf("exited (code %d)", r.Stop.ExitCode))
		} else {
			msg = dimCyan("exited (code 0)")
		}
		_, _ = fmt.Fprintf(w, "%s%s\n", prefix(r.Stop.TS, dim("│")), msg)
	case "stats":
		var parts []string
		for _, s := range r.Stats.Stats {
			parts = append(parts, dim(s.Label+"=")+s.Value)
		}
		_, _ = fmt.Fprintf(w, "%s%s\n", prefix(r.Stats.TS, dim("~")), dim(strings.Join(parts, "  ")))
	case "log":
		msg := r.Log.Msg
		if r.Log.Stream == "stderr" {
			msg = red(msg)
		}
		_, _ = fmt.Fprintf(w, "%s%s\n", prefix(r.Log.TS, " "), msg)
	}
}

type logPrinter struct {
	lastDate   string
	timeFormat string // resolved Go layout, or "" for none
}

func resolveTimeFormat(s string) string {
	switch s {
	case "time":
		return "15:04:05.000"
	case "datetime":
		return "2006-01-02 15:04:05.000"
	case "none":
		return ""
	default:
		return s
	}
}

func (p *logPrinter) fmtTS(s string) string {
	if p.timeFormat == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Local().Format(p.timeFormat)
	}
	return s
}
