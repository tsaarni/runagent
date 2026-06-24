// Defines event types and JSON Lines log file I/O for recording process lifecycle and output.

package runagent

import (
	"bufio"
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
type EventReader struct {
	scanner *bufio.Scanner
}

func NewEventReader(r io.Reader) *EventReader {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1024*1024), 1024*1024)
	return &EventReader{scanner: s}
}

// Next reads the next event line. Returns the raw line and decoded header.
// Returns io.EOF when no more events are available.
func (er *EventReader) Next() (EventHeader, json.RawMessage, error) {
	if !er.scanner.Scan() {
		if err := er.scanner.Err(); err != nil {
			return EventHeader{}, nil, err
		}
		return EventHeader{}, nil, io.EOF
	}
	line := er.scanner.Bytes()
	raw := make(json.RawMessage, len(line))
	copy(raw, line)
	var h EventHeader
	if err := json.Unmarshal(line, &h); err != nil {
		return EventHeader{}, raw, err
	}
	return h, raw, nil
}
