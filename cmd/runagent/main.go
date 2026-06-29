// CLI entrypoint that parses commands and dispatches to the daemon or subcommand handlers.

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/alecthomas/kong"

	"github.com/tsaarni/runagent/daemon"
	"github.com/tsaarni/runagent"
)

var cli struct {
	Output string `help:"Output format (table or json)." default:"table" enum:"table,json"`
	JSON   bool   `help:"Shorthand for --output=json." name:"json"`

	Run      RunCmd      `cmd:"" aliases:"start" help:"Spawn a background process."`
	List     ListCmd     `cmd:"" aliases:"ps" help:"List all managed processes."`
	Status   StatusCmd   `cmd:"" aliases:"stats" help:"Show detailed process metrics."`
	Logs     LogsCmd     `cmd:"" help:"Read process log output."`
	Kill     KillCmd     `cmd:"" help:"Send signal to process (default: SIGTERM)."`
	Delete   DeleteCmd   `cmd:"" aliases:"rm" help:"Remove process and logs."`
	Wait     WaitCmd     `cmd:"" help:"Block until process exits."`
	Shutdown ShutdownCmd `cmd:"" help:"Gracefully stop daemon and all processes."`
	Daemon   DaemonCmd   `cmd:"" help:"Manage the daemon."`

	DaemonRun DaemonRunCmd `cmd:"" hidden:"" name:"daemon-run"`
}

type RunCmd struct {
	Name    string   `help:"Process name." short:"n"`
	Env     []string `help:"Environment variable KEY=VALUE (repeatable)." short:"e"`
	Cwd     string   `help:"Working directory." type:"path"`
	Command []string `arg:"" required:"" passthrough:"" help:"Command and arguments to run."`
}
type ListCmd struct{}
type StatusCmd struct {
	Target string `arg:"" optional:"" help:"Process name or ID (omit for all)."`
}
type LogsCmd struct {
	Type      string `help:"Event types to show (comma-separated: start,log,stats,stop)." default:"start,log,stats,stop"`
	TimeRange string `help:"Time range (FROM..TO). FROM/TO: -5m (ago), 21:00:00, 2026-06-24T21:00:00, +2m (relative to FROM)." name:"time-range"`
	Limit     int    `help:"Max events to return (first N from window)." default:"0"`
	Last      int    `help:"Return last N matching events." default:"0"`
	TimeFormat string `help:"Timestamp format: time, datetime, none, or Go layout string." default:"time" name:"time-format"`
	Follow     bool   `help:"Follow log output." short:"f"`
	Stream    string `help:"Filter by stream (stdout or stderr)."`
	Target    string `arg:"" help:"Process name or ID."`
}
type KillCmd struct {
	Signal string `help:"Signal to send." default:"SIGTERM"`
	Target string `arg:"" help:"Process name or ID."`
}
type DeleteCmd struct {
	All    bool   `help:"Delete all non-running processes."`
	Force  bool   `help:"Kill running process and delete."`
	Target string `arg:"" optional:"" help:"Process name or ID."`
}
type WaitCmd struct {
	Timeout string `help:"Timeout duration (e.g., 30s)."`
	Target  string `arg:"" help:"Process name or ID."`
}
type ShutdownCmd struct{}
type DaemonCmd struct {
	Status DaemonStatusCmd `cmd:"" default:"1" help:"Show daemon status."`
	Start  DaemonStartCmd  `cmd:"" help:"Start the daemon."`
	Stop   DaemonStopCmd   `cmd:"" help:"Stop the daemon."`
	Clean  DaemonCleanCmd  `cmd:"" help:"Remove all runtime and state files."`
}
type DaemonStatusCmd struct{}
type DaemonStartCmd struct{}
type DaemonStopCmd struct{}
type DaemonCleanCmd struct{}
type DaemonRunCmd struct{}

func main() {
	helpWithHint := func(options kong.HelpOptions, ctx *kong.Context) error {
		if hasArg(ctx.Args, "--output=json", "--json") {
			printHelpJSON()
			ctx.Kong.Exit(0)
			return nil
		}
		if err := kong.DefaultHelpPrinter(options, ctx); err != nil {
			return err
		}
		if ctx.Selected() == nil {
			fmt.Fprintln(ctx.Stdout, "\nFor full help of all commands in machine-readable format: runagent --help --output=json") //nolint:errcheck
		}
		return nil
	}
	ctx := kong.Parse(&cli, kong.UsageOnError(), kong.Help(helpWithHint))
	if cli.JSON {
		cli.Output = "json"
	}
	switch ctx.Command() {
	case "daemon-run":
		runDaemon()
	case "daemon status", "daemon":
		cmdDaemonStatus()
	case "daemon start":
		cmdDaemonStart()
	case "daemon stop":
		cmdDaemonStop()
	case "daemon clean":
		cmdDaemonClean()
	case "run <command>", "start <command>":
		cmdRun()
	case "list", "ps":
		cmdList()
	case "status", "status <target>", "stats", "stats <target>":
		cmdStatus()
	case "logs <target>":
		cmdLogs()
	case "kill <target>":
		cmdKill()
	case "delete", "delete <target>", "rm", "rm <target>":
		cmdDelete()
	case "wait <target>":
		cmdWait()
	case "shutdown":
		cmdShutdown()
	default:
		ctx.FatalIfErrorf(fmt.Errorf("unknown command: %s", ctx.Command()))
	}
}

// --- Infrastructure ---

func runDaemon() {
	rtDir := runtimeDir()
	stDir := stateDir()
	if err := os.MkdirAll(rtDir, 0700); err != nil {
		fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(stDir, 0700); err != nil {
		fatalf("mkdir: %v", err)
	}

	logPath := filepath.Join(stDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	}

	pidPath := filepath.Join(rtDir, "daemon.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		fatalf("write pid: %v", err)
	}

	if err := daemon.Run(rtDir, stDir); err != nil {
		slog.Error("daemon failed", "error", err)
		os.Exit(1)
	}
}

func runtimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "runagent")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("runagent-%d", os.Getuid()))
}

func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "runagent")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "runagent")
}

func ensureDaemon() net.Conn {
	rtDir := runtimeDir()
	if err := os.MkdirAll(rtDir, 0700); err != nil {
		fatalf("mkdir: %v", err)
	}

	conn, err := runagent.Dial(rtDir)
	if err == nil {
		return conn
	}

	lockPath := filepath.Join(rtDir, "daemon.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err == nil {
		err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "daemon-run")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			_ = cmd.Start()
			_ = lockFile.Close()
		} else {
			_ = lockFile.Close()
		}
	}

	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		conn, err = runagent.Dial(rtDir)
		if err == nil {
			return conn
		}
	}
	fatalf("cannot connect to daemon")
	return nil
}

func sendRecv(req *runagent.Request) *runagent.Response {
	conn := ensureDaemon()
	defer func() { _ = conn.Close() }()
	if err := runagent.Send(conn, req); err != nil {
		fatalf("send: %v", err)
	}
	var resp runagent.Response
	if err := runagent.Recv(conn, &resp); err != nil {
		fatalf("recv: %v", err)
	}
	return &resp
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if cli.Output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"ok": false, "error": msg, "data": nil})
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", boldRed("error:"), msg)
	}
	os.Exit(1)
}

func checkResp(resp *runagent.Response) {
	if !resp.OK {
		fatalf("%s", resp.Error)
	}
}

func mustArgs(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func printJSON(resp *runagent.Response) {
	out := map[string]any{"ok": true, "error": "", "data": json.RawMessage(resp.Data)}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func formatDuration(startedAt, exitedAt string) string {
	t, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return "-"
	}
	end := time.Now()
	if exitedAt != "" {
		if e, err := time.Parse(time.RFC3339Nano, exitedAt); err == nil {
			end = e
		}
	}
	d := end.Sub(t).Truncate(time.Second)
	if d <= 0 {
		return "0s"
	}
	return d.String()
}

func formatTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func toStringSlice(v any) []string {
	if arr, ok := v.([]any); ok {
		s := make([]string, len(arr))
		for i, a := range arr {
			s[i] = fmt.Sprint(a)
		}
		return s
	}
	return nil
}


func hasArg(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}
