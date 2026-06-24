// Implements daemon lifecycle subcommands (status, start, stop, clean).

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tsaarni/runagent"
)

func cmdDaemonStatus() {
	rtDir := runtimeDir()
	stDir := stateDir()
	pidPath := rtDir + "/daemon.pid"
	sockPath := rtDir + "/daemon.sock"
	logPath := stDir + "/daemon.log"

	running := false
	pid := 0
	if data, err := os.ReadFile(pidPath); err == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			pid = p
			if syscall.Kill(pid, 0) == nil {
				running = true
			}
		}
	}

	if cli.Output == "json" {
		out := map[string]any{
			"ok": true, "error": "",
			"data": map[string]any{
				"running": running, "pid": pid,
				"runtime_dir": rtDir, "state_dir": stDir,
				"socket": sockPath, "log_file": logPath, "pid_file": pidPath,
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	kv := newProps()
	if running {
		kv.add("Status:", green("running"))
		kv.add("PID:", strconv.Itoa(pid))
	} else {
		kv.add("Status:", dim("not running"))
	}
	kv.add("Runtime dir:", rtDir)
	kv.add("State dir:", stDir)
	kv.add("Socket:", sockPath)
	kv.add("Log file:", logPath)
	kv.add("PID file:", pidPath)
	kv.print()
}

func cmdDaemonStart() {
	rtDir := runtimeDir()
	pidPath := rtDir + "/daemon.pid"

	if data, err := os.ReadFile(pidPath); err == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if syscall.Kill(p, 0) == nil {
				fmt.Printf("Daemon already running (pid %d)\n", p)
				return
			}
		}
	}

	if err := os.MkdirAll(rtDir, 0700); err != nil {
		fatalf("%v", err)
	}
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "daemon-run")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fatalf("%v", err)
	}

	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		if conn, err := runagent.Dial(rtDir); err == nil {
			_ = conn.Close()
			fmt.Printf("%s Daemon started %s\n", successIcon(), dim(fmt.Sprintf("pid=%d", cmd.Process.Pid)))
			return
		}
	}
	fatalf("daemon did not start in time")
}

func cmdDaemonStop() {
	rtDir := runtimeDir()
	pidPath := rtDir + "/daemon.pid"

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println(dim("Daemon is not running"))
		return
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || syscall.Kill(p, 0) != nil {
		fmt.Println(dim("Daemon is not running"))
		return
	}

	if conn, err := runagent.Dial(rtDir); err == nil {
		_ = runagent.Send(conn, &runagent.Request{Command: "shutdown", Args: json.RawMessage("{}")})
		var resp runagent.Response
		_ = runagent.Recv(conn, &resp)
		_ = conn.Close()
	}
	fmt.Printf("%s Daemon stopping\n", successIcon())
}

func cmdDaemonClean() {
	rtDir := runtimeDir()
	stDir := stateDir()

	_ = os.RemoveAll(rtDir)
	_ = os.RemoveAll(stDir)
	fmt.Printf("%s Removed %s\n", successIcon(), dim(rtDir))
	fmt.Printf("%s Removed %s\n", successIcon(), dim(stDir))
}
