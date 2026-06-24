// Implements the background daemon that spawns, monitors, and manages child processes.

package daemon

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tsaarni/runagent"
)

type Daemon struct {
	runtimeDir string
	stateDir   string
	registry   *Registry
	mu         sync.Mutex
	listener   net.Listener
	waitChans  map[int][]chan struct{} // process ID -> channels waiting for exit
	logFiles   map[int]*runagent.LogFile      // process ID -> open log file
	collectors map[int]*StatsCollector       // process ID -> stats collector
}

func Run(runtimeDir, stateDir string) error {
	logsDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		return fmt.Errorf("mkdir logs: %w", err)
	}

	// Acquire flock
	lockPath := filepath.Join(runtimeDir, "daemon.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open lock: %w", err)
	}
	defer func() { _ = lockFile.Close() }()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("another daemon is running")
	}

	// Crash recovery: wipe registry and logs
	regPath := filepath.Join(stateDir, "registry.json")
	_ = os.Remove(regPath)
	entries, _ := os.ReadDir(logsDir)
	for _, e := range entries {
		_ = os.Remove(filepath.Join(logsDir, e.Name()))
	}

	reg := NewRegistry(regPath)

	sockPath := filepath.Join(runtimeDir, "daemon.sock")
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	d := &Daemon{
		runtimeDir: runtimeDir,
		stateDir:   stateDir,
		registry:   reg,
		listener:   ln,
		waitChans:  make(map[int][]chan struct{}),
		logFiles:   make(map[int]*runagent.LogFile),
		collectors: make(map[int]*StatsCollector),
	}

	// Signal handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		d.shutdown()
	}()

	// Stats poller
	go d.pollStats()

	slog.Info("daemon listening", "socket", sockPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return nil // listener closed
		}
		go d.handle(conn)
	}
}

func (d *Daemon) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	var req runagent.Request
	if err := runagent.Recv(conn, &req); err != nil {
		return
	}
	switch req.Command {
	case "start":
		d.handleStart(conn, req.Args)
	case "list":
		d.handleList(conn)
	case "status":
		d.handleStatus(conn, req.Args)
	case "logs":
		d.handleLogs(conn, req.Args)
	case "kill":
		d.handleKill(conn, req.Args)
	case "delete":
		d.handleDelete(conn, req.Args)
	case "wait":
		d.handleWait(conn, req.Args)
	case "shutdown":
		_ = runagent.SendOK(conn, struct{}{})
		go d.shutdown()
	default:
		_ = runagent.SendError(conn, "unknown command: "+req.Command)
	}
}

func (d *Daemon) handleStart(conn net.Conn, raw json.RawMessage) {
	var args runagent.StartArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		_ = runagent.SendError(conn, "invalid args: "+err.Error())
		return
	}
	if len(args.Command) == 0 {
		_ = runagent.SendError(conn, "command is required")
		return
	}

	name := args.Name
	if name == "" {
		name = d.generateName(args.Command[0])
	}

	d.mu.Lock()
	if d.registry.FindByName(name) != nil {
		d.mu.Unlock()
		_ = runagent.SendError(conn, fmt.Sprintf("process %q already exists", name))
		return
	}

	id := newUUID()
	p := &Process{
		UUID:      id,
		Name:      name,
		Command:   args.Command,
		Env:       args.Env,
		Cwd:       args.Cwd,
		State:     Running,
		StartedAt: time.Now(),
	}
	d.registry.Add(p)
	d.mu.Unlock()

	// Spawn
	logPath := filepath.Join(d.stateDir, "logs", id+".log")
	logFile, err := runagent.CreateLogFile(logPath)
	if err != nil {
		d.mu.Lock()
		d.registry.Remove(p.ID)
		_ = d.registry.Save()
		d.mu.Unlock()
		_ = runagent.SendError(conn, "create log file: "+err.Error())
		return
	}

	cmd := exec.Command(args.Command[0], args.Command[1:]...)
	cmd.Dir = args.Cwd
	if len(args.Env) > 0 {
		cmd.Env = append(os.Environ(), args.Env...)
	}
	cmd.SysProcAttr = sysProcAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = os.Remove(logPath)
		d.mu.Lock()
		d.registry.Remove(p.ID)
		_ = d.registry.Save()
		d.mu.Unlock()
		_ = runagent.SendError(conn, "start: "+err.Error())
		return
	}

	d.mu.Lock()
	p.PID = cmd.Process.Pid
	// Read starttime from /proc
	if st, err := ReadStartTime(p.PID); err == nil {
		p.StartTime = st
	}
	_ = d.registry.Save()
	d.mu.Unlock()

	// Write start event
	_ = logFile.Append(runagent.StartEvent{
		EventHeader: runagent.EventHeader{Type: "start", TS: p.StartedAt.Format(time.RFC3339Nano)},
		ID: p.ID, Name: p.Name, UUID: p.UUID,
		Command: p.Command, Cwd: p.Cwd, Env: p.Env,
	})

	d.mu.Lock()
	d.logFiles[p.ID] = logFile
	d.mu.Unlock()

	// Log routing goroutines
	var wg sync.WaitGroup
	wg.Add(2)
	go d.routeLog(stdout, logFile, "stdout", &wg)
	go d.routeLog(stderr, logFile, "stderr", &wg)

	// Wait goroutine
	go func() {
		wg.Wait()
		err := cmd.Wait()
		_ = logFile.Sync()

		d.mu.Lock()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					p.State = Crashed
					p.Signal = int(ws.Signal())
				} else {
					p.State = Exited
					p.ExitCode = exitErr.ExitCode()
				}
			} else {
				p.State = Crashed
			}
		} else {
			p.State = Exited
			p.ExitCode = 0
		}
		p.ExitedAt = time.Now()
		_ = d.registry.Save()

		// Notify waiters
		for _, ch := range d.waitChans[p.ID] {
			close(ch)
		}
		delete(d.waitChans, p.ID)
		delete(d.logFiles, p.ID)
		d.mu.Unlock()

		// Write stop event
		_ = logFile.Append(runagent.StopEvent{
			EventHeader: runagent.EventHeader{Type: "stop", TS: time.Now().Format(time.RFC3339Nano)},
			State: string(p.State), ExitCode: p.ExitCode, Signal: p.Signal,
		})
		_ = logFile.Close()
	}()

	_ = runagent.SendOK(conn, map[string]any{"id": p.ID, "name": p.Name, "uuid": p.UUID, "pid": p.PID})
}

func (d *Daemon) routeLog(r io.Reader, logFile *runagent.LogFile, stream string, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		rec := runagent.LogEvent{
			EventHeader: runagent.EventHeader{Type: "log", TS: time.Now().Format(time.RFC3339Nano)},
			Stream: stream, Msg: scanner.Text(),
		}
		_ = logFile.Append(rec)
	}
}


func (d *Daemon) handleList(conn net.Conn) {
	d.mu.Lock()
	procs := d.registry.All()
	d.mu.Unlock()

	type entry struct {
		ID        int      `json:"id"`
		Name      string   `json:"name"`
		PID       int      `json:"pid"`
		State     string   `json:"state"`
		Command   []string `json:"command"`
		ExitCode  int      `json:"exit_code"`
		Signal    int      `json:"signal"`
		StartedAt string   `json:"started_at"`
		ExitedAt  string   `json:"exited_at,omitempty"`
	}
	list := make([]entry, 0, len(procs))
	for _, p := range procs {
		e := entry{
			ID: p.ID, Name: p.Name, PID: p.PID, State: string(p.State),
			Command: p.Command, ExitCode: p.ExitCode, Signal: p.Signal,
			StartedAt: p.StartedAt.Format(time.RFC3339Nano),
		}
		if !p.ExitedAt.IsZero() {
			e.ExitedAt = p.ExitedAt.Format(time.RFC3339Nano)
		}
		list = append(list, e)
	}
	_ = runagent.SendOK(conn, list)
}

func (d *Daemon) handleStatus(conn net.Conn, raw json.RawMessage) {
	var args runagent.StatusArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		_ = runagent.SendError(conn, "invalid args")
		return
	}
	d.mu.Lock()
	p := d.registry.Resolve(args.Target)
	if p == nil {
		d.mu.Unlock()
		_ = runagent.SendError(conn, "process not found: "+args.Target)
		return
	}

	result := map[string]any{
		"id": p.ID, "name": p.Name, "pid": p.PID, "state": string(p.State),
		"command": p.Command, "exit_code": p.ExitCode, "signal": p.Signal,
		"started_at": p.StartedAt.Format(time.RFC3339Nano),
	}
	if !p.ExitedAt.IsZero() {
		result["exited_at"] = p.ExitedAt.Format(time.RFC3339Nano)
	}

	if p.State == Running {
		sc := d.collectors[p.ID]
		if sc == nil {
			sc = NewStatsCollector(p.PID)
			d.collectors[p.ID] = sc
		}
		pid := p.PID
		startTime := p.StartTime
		d.mu.Unlock()

		// Verify PID and collect stats outside lock
		st, err := ReadStartTime(pid)
		if err != nil || st != startTime {
			result["error"] = "PID recycled or process gone"
		} else {
			stats, err := sc.Collect(Snapshot)
			if err == nil {
				d.mu.Lock()
				p.LastStats = stats
				d.mu.Unlock()
				result["stats"] = stats
			}
		}
	} else {
		if p.LastStats != nil {
			result["stats"] = p.LastStats
		}
		d.mu.Unlock()
	}

	_ = runagent.SendOK(conn, result)
}

func (d *Daemon) handleLogs(conn net.Conn, raw json.RawMessage) {
	var args runagent.LogsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		_ = runagent.SendError(conn, "invalid args")
		return
	}
	d.mu.Lock()
	p := d.registry.Resolve(args.Target)
	d.mu.Unlock()
	if p == nil {
		_ = runagent.SendError(conn, "process not found: "+args.Target)
		return
	}
	logPath := filepath.Join(d.stateDir, "logs", p.UUID+".log")
	_ = runagent.SendOK(conn, map[string]any{"path": logPath})
}

func (d *Daemon) handleKill(conn net.Conn, raw json.RawMessage) {
	var args runagent.KillArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		_ = runagent.SendError(conn, "invalid args")
		return
	}
	d.mu.Lock()
	p := d.registry.Resolve(args.Target)
	d.mu.Unlock()
	if p == nil {
		_ = runagent.SendError(conn, "process not found: "+args.Target)
		return
	}
	if p.State != Running {
		_ = runagent.SendError(conn, "process is not running")
		return
	}

	// Verify PID
	st, err := ReadStartTime(p.PID)
	if err != nil || st != p.StartTime {
		_ = runagent.SendError(conn, "process already exited, PID recycled")
		return
	}

	sig := parseSignal(args.Signal)
	if sig == 0 {
		_ = runagent.SendError(conn, "invalid signal: "+args.Signal)
		return
	}
	_ = syscall.Kill(-p.PID, sig) // kill process group
	_ = runagent.SendOK(conn, map[string]any{"id": p.ID, "name": p.Name})
}

func (d *Daemon) handleDelete(conn net.Conn, raw json.RawMessage) {
	var args runagent.DeleteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		_ = runagent.SendError(conn, "invalid args")
		return
	}

	d.mu.Lock()
	var targets []*Process
	if args.All {
		for _, p := range d.registry.All() {
			if args.Force || p.State != Running {
				targets = append(targets, p)
			}
		}
	} else {
		p := d.registry.Resolve(args.Target)
		if p == nil {
			d.mu.Unlock()
			_ = runagent.SendError(conn, "process not found: "+args.Target)
			return
		}
		if !args.Force && p.State == Running {
			d.mu.Unlock()
			_ = runagent.SendError(conn, "process is still running, use --force to kill and delete")
			return
		}
		targets = []*Process{p}
	}

	// Kill running processes and register wait channels
	var waitChs []chan struct{}
	for _, p := range targets {
		if p.State == Running {
			_ = syscall.Kill(-p.PID, syscall.SIGKILL)
			ch := make(chan struct{})
			d.waitChans[p.ID] = append(d.waitChans[p.ID], ch)
			waitChs = append(waitChs, ch)
		}
	}
	d.mu.Unlock()

	// Wait for killed processes to exit
	for _, ch := range waitChs {
		<-ch
	}

	// Now all targets are in terminal state, safe to delete
	d.mu.Lock()
	for _, p := range targets {
		logPath := filepath.Join(d.stateDir, "logs", p.UUID+".log")
		_ = os.Remove(logPath)
		delete(d.collectors, p.ID)
		delete(d.logFiles, p.ID)
		d.registry.Remove(p.ID)
	}
	_ = d.registry.Save()
	d.mu.Unlock()

	if args.All {
		_ = runagent.SendOK(conn, map[string]any{"deleted": len(targets)})
	} else if len(targets) > 0 {
		_ = runagent.SendOK(conn, map[string]any{"id": targets[0].ID, "name": targets[0].Name})
	} else {
		_ = runagent.SendOK(conn, map[string]any{})
	}
}

func (d *Daemon) handleWait(conn net.Conn, raw json.RawMessage) {
	var args runagent.WaitArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		_ = runagent.SendError(conn, "invalid args")
		return
	}
	d.mu.Lock()
	p := d.registry.Resolve(args.Target)
	if p == nil {
		d.mu.Unlock()
		_ = runagent.SendError(conn, "process not found: "+args.Target)
		return
	}

	// Already terminal?
	if p.State != Running {
		d.mu.Unlock()
		_ = runagent.SendOK(conn, map[string]any{"id": p.ID, "name": p.Name, "exit_code": p.ExitCode, "state": string(p.State)})
		return
	}

	// Register waiter
	ch := make(chan struct{})
	d.waitChans[p.ID] = append(d.waitChans[p.ID], ch)
	id := p.ID
	d.mu.Unlock()

	var timeout <-chan time.Time
	if args.Timeout != "" {
		dur, err := time.ParseDuration(args.Timeout)
		if err != nil {
			_ = runagent.SendError(conn, "invalid timeout: "+args.Timeout)
			return
		}
		timeout = time.After(dur)
	}

	// Detect client disconnect
	disconnected := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		_, _ = conn.Read(buf) // blocks until connection closes
		close(disconnected)
	}()

	select {
	case <-ch:
		d.mu.Lock()
		p = d.registry.Get(id)
		d.mu.Unlock()
		if p == nil {
			_ = runagent.SendError(conn, "process was deleted")
			return
		}
		_ = runagent.SendOK(conn, map[string]any{"id": p.ID, "name": p.Name, "exit_code": p.ExitCode, "state": string(p.State)})
	case <-timeout:
		d.removeWaitChan(id, ch)
		_ = runagent.SendError(conn, "timeout waiting for process")
	case <-disconnected:
		d.removeWaitChan(id, ch)
	}
}

func (d *Daemon) removeWaitChan(id int, ch chan struct{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	chans := d.waitChans[id]
	for i, c := range chans {
		if c == ch {
			d.waitChans[id] = append(chans[:i], chans[i+1:]...)
			break
		}
	}
	if len(d.waitChans[id]) == 0 {
		delete(d.waitChans, id)
	}
}

func (d *Daemon) pollStats() {
	lastEmitted := make(map[int]runagent.Stats)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// Gather running process info under lock
		type pollTarget struct {
			id int
			sc *StatsCollector
		}
		d.mu.Lock()
		var targets []pollTarget
		for _, p := range d.registry.All() {
			if p.State != Running {
				delete(d.collectors, p.ID)
				delete(lastEmitted, p.ID)
				continue
			}
			sc := d.collectors[p.ID]
			if sc == nil {
				sc = NewStatsCollector(p.PID)
				d.collectors[p.ID] = sc
			}
			targets = append(targets, pollTarget{id: p.ID, sc: sc})
		}
		d.mu.Unlock()

		// Collect stats without holding lock (I/O)
		type pollResult struct {
			id    int
			stats runagent.Stats
		}
		var results []pollResult
		for _, t := range targets {
			stats, err := t.sc.Collect(Sample)
			if err == nil {
				results = append(results, pollResult{id: t.id, stats: stats})
			}
		}

		// Apply results under lock
		d.mu.Lock()
		for _, r := range results {
			p := d.registry.Get(r.id)
			if p == nil || p.State != Running {
				continue
			}
			p.LastStats = r.stats
			if !statsEqual(r.stats, lastEmitted[r.id]) {
				if lf := d.logFiles[r.id]; lf != nil {
					_ = lf.Append(runagent.StatsEvent{
						EventHeader: runagent.EventHeader{Type: "stats", TS: time.Now().Format(time.RFC3339Nano)},
						Stats:       r.stats,
					})
				}
				lastEmitted[r.id] = r.stats
			}
		}
		_ = d.registry.Save()
		d.mu.Unlock()
	}
}

func statsEqual(a, b runagent.Stats) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (d *Daemon) shutdown() {
	d.mu.Lock()
	for _, p := range d.registry.All() {
		if p.State == Running {
			_ = syscall.Kill(-p.PID, syscall.SIGKILL) // kill process group
		}
	}
	d.mu.Unlock()

	time.Sleep(500 * time.Millisecond)
	_ = d.listener.Close()
	_ = os.Remove(filepath.Join(d.runtimeDir, "daemon.sock"))
	_ = os.Remove(filepath.Join(d.runtimeDir, "daemon.pid"))
	os.Exit(0)
}

func (d *Daemon) generateName(bin string) string {
	base := filepath.Base(bin)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.registry.FindByName(base) == nil {
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s-%d", base, i)
		if d.registry.FindByName(name) == nil {
			return name
		}
	}
}

func parseSignal(s string) syscall.Signal {
	if s == "" {
		return syscall.SIGKILL
	}
	s = strings.ToUpper(s)
	if !strings.HasPrefix(s, "SIG") {
		s = "SIG" + s
	}
	signals := map[string]syscall.Signal{
		"SIGHUP": syscall.SIGHUP, "SIGINT": syscall.SIGINT, "SIGQUIT": syscall.SIGQUIT,
		"SIGKILL": syscall.SIGKILL, "SIGTERM": syscall.SIGTERM, "SIGUSR1": syscall.SIGUSR1,
		"SIGUSR2": syscall.SIGUSR2, "SIGCONT": syscall.SIGCONT, "SIGSTOP": syscall.SIGSTOP,
	}
	if sig, ok := signals[s]; ok {
		return sig
	}
	// Try numeric
	s = strings.TrimPrefix(s, "SIG")
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n)
	}
	return 0
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
