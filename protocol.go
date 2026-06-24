// Implements the length-prefixed JSON message protocol for CLI-to-daemon communication over Unix sockets.

package runagent

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
)

const maxMessageSize = 1 << 20 // 1MB

type Request struct {
	Command string          `json:"command"`
	Args    json.RawMessage `json:"args"`
}

type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type StartArgs struct {
	Command []string `json:"command"`
	Name    string   `json:"name"`
	Env     []string `json:"env"`
	Cwd     string   `json:"cwd"`
}

type KillArgs struct {
	Target string `json:"target"`
	Signal string `json:"signal"`
}

type DeleteArgs struct {
	Target string `json:"target"`
	All    bool   `json:"all"`
	Force  bool   `json:"force"`
}

type LogsArgs struct {
	Target string `json:"target"`
}

type StatusArgs struct {
	Target string `json:"target"`
}

type WaitArgs struct {
	Target  string `json:"target"`
	Timeout string `json:"timeout"`
}

func Send(conn net.Conn, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(data) > maxMessageSize {
		return fmt.Errorf("message too large: %d bytes", len(data))
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err = conn.Write(data)
	return err
}

func Recv(conn net.Conn, out any) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header)
	if size > maxMessageSize {
		return fmt.Errorf("message too large: %d bytes", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func Dial(runtimeDir string) (net.Conn, error) {
	return net.Dial("unix", filepath.Join(runtimeDir, "daemon.sock"))
}

func SendOK(conn net.Conn, data any) error {
	var raw json.RawMessage
	if data != nil {
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}
	return Send(conn, Response{OK: true, Data: raw})
}

func SendError(conn net.Conn, msg string) error {
	return Send(conn, Response{OK: false, Error: msg})
}
