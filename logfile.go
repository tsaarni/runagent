// Defines the schema for log files stored at <state_dir>/logs/<uuid>.log.
// Each log file is JSON Lines: one JSON object per line, using the event types below.
// Also provides LogFile (writer) and EventReader (reader) for log file I/O.

package runagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"
)

type EventHeader struct {
	Type string `json:"type"`
	TS   string `json:"ts"`
}

type StartEvent struct {
	EventHeader
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	UUID    string   `json:"uuid"`
	Command []string `json:"command"`
	Cwd     string   `json:"cwd"`
	Env     []string `json:"env"`
}

type StopEvent struct {
	EventHeader
	State    string `json:"state"`
	ExitCode int    `json:"exit_code,omitempty"`
	Signal   int    `json:"signal,omitempty"`
}

type LogEvent struct {
	EventHeader
	Stream string `json:"stream"`
	Msg    string `json:"msg"`
}

type StatsEvent struct {
	EventHeader
	Stats Stats `json:"stats"`
}

// Stats is an ordered list of display-formatted key-value metrics.
type Stats []Stat

type Stat struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// LogFile provides serialized append access to a JSON Lines log file.
type LogFile struct {
	f  *os.File
	mu sync.Mutex
}

func CreateLogFile(path string) (*LogFile, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &LogFile{f: f}, nil
}

func (lf *LogFile) Append(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	lf.mu.Lock()
	_, err = lf.f.Write(data)
	lf.mu.Unlock()
	return err
}

func (lf *LogFile) Sync() error {
	return lf.f.Sync()
}

func (lf *LogFile) Close() error {
	return lf.f.Close()
}

// EventReader reads events from a JSON Lines stream.
// Safe to call Next() again after EOF when more data is appended (tail mode).
type EventReader struct {
	r *bufio.Reader
}

func NewEventReader(r io.Reader) *EventReader {
	return &EventReader{r: bufio.NewReader(r)}
}

// Next reads the next event line. Returns io.EOF when no complete line
// is available. Safe to call again after more data is appended.
func (er *EventReader) Next() (EventHeader, json.RawMessage, error) {
	line, err := er.r.ReadBytes('\n')
	if err != nil {
		return EventHeader{}, nil, err
	}
	line = bytes.TrimSuffix(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	raw := make(json.RawMessage, len(line))
	copy(raw, line)
	var h EventHeader
	if err := json.Unmarshal(line, &h); err != nil {
		return EventHeader{}, raw, err
	}
	return h, raw, nil
}
