// Parses "FROM..TO" time range expressions for log filtering (durations, clock times, RFC3339).

package main

import (
	"fmt"
	"strings"
	"time"
)

type TimeRange struct {
	From *time.Time // nil = beginning
	To   *time.Time // nil = end
}

// parseTimeRange parses a "FROM..TO" expression.
// FROM: -5m (ago), HH:MM:SS (today), RFC3339, or empty (beginning).
// TO: -5m (ago), +2m (relative to FROM), HH:MM:SS, RFC3339, or empty (end).
func parseTimeRange(expr string, now time.Time) (TimeRange, error) {
	parts := strings.SplitN(expr, "..", 2)
	if len(parts) != 2 {
		return TimeRange{}, fmt.Errorf("invalid time-range %q: expected FROM..TO", expr)
	}
	fromStr, toStr := parts[0], parts[1]

	var tr TimeRange

	if fromStr != "" {
		t, err := parseTimeExpr(fromStr, now, nil)
		if err != nil {
			return TimeRange{}, fmt.Errorf("invalid FROM in time-range: %w", err)
		}
		tr.From = &t
	}

	if toStr != "" {
		t, err := parseTimeExpr(toStr, now, tr.From)
		if err != nil {
			return TimeRange{}, fmt.Errorf("invalid TO in time-range: %w", err)
		}
		tr.To = &t
	}

	return tr, nil
}

// parseTimeExpr parses a single time expression.
// - "-5m" means duration ago from now
// - "+2m" means duration after anchor (only valid for TO with a FROM)
// - "15:04:05" means time today
// - RFC3339 or "2006-01-02T15:04:05"
func parseTimeExpr(s string, now time.Time, anchor *time.Time) (time.Time, error) {
	if strings.HasPrefix(s, "+") {
		d, err := time.ParseDuration(s[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid offset %q: %w", s, err)
		}
		if anchor == nil {
			return time.Time{}, fmt.Errorf("offset %q requires a FROM value", s)
		}
		return anchor.Add(d), nil
	}

	if strings.HasPrefix(s, "-") {
		d, err := time.ParseDuration(s[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return now.Add(-d), nil
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	// Try local datetime without timezone
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local); err == nil {
		return t, nil
	}
	// Try HH:MM:SS (today)
	if t, err := time.ParseInLocation("15:04:05", s, time.Local); err == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
		return today, nil
	}
	// Try HH:MM
	if t, err := time.ParseInLocation("15:04", s, time.Local); err == nil {
		today := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
		return today, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse time %q", s)
}
