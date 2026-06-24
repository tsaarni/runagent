// Defines the Process struct and state/mode constants used across the daemon.

package daemon

import (
	"time"

	"github.com/tsaarni/runagent"
)

type State string

const (
	Running State = "Running"
	Exited  State = "Exited"
	Crashed State = "Crashed"
)

type StatsMode int

const (
	Snapshot StatsMode = iota // full detail for live display
	Sample                    // stable metrics for time-series logging
)

type Process struct {
	ID        int            `json:"id"`
	UUID      string         `json:"uuid"`
	Name      string         `json:"name"`
	Command   []string       `json:"command"`
	Env       []string       `json:"env"`
	Cwd       string         `json:"cwd"`
	PID       int            `json:"pid"`
	StartTime int64          `json:"start_time"`
	State     State          `json:"state"`
	ExitCode  int            `json:"exit_code"`
	Signal    int            `json:"signal"`
	StartedAt time.Time      `json:"started_at"`
	ExitedAt  time.Time      `json:"exited_at,omitempty"`
	LastStats runagent.Stats `json:"last_stats,omitempty"`
}
