<p align="center">
  <img src="assets/runagent.svg" alt="runagent" width="240">
</p>

Background process manager for agents.

When using LLM agent to debug and troubleshoot applications, it typically uses `myapp &` and then `ps | grep` to check the process.
This approach is fragile:

- The agent may lose track of the process or match the wrong one
- If the process exits with an error, the agent may not notice and keeps trying to interact with it, wasting time on troubleshooting
- The agent may forget to capture stdout/stderr, forcing a full restart just to collect logs
- Processes may be left running after the agent session ends

`runagent` solves these by providing means for the agent to start and manage processes in a robust way.


## Install

Install the binary:

```console
go install github.com/tsaarni/runagent/cmd/runagent@latest
```

Or run directly without installing:

```console
go run github.com/tsaarni/runagent/cmd/runagent@latest run -n myapp -- myapp
```

## Usage

| Command | Description |
|---------|-------------|
| `run` | Spawn a background process |
| `ps` | List all processes or show process details |
| `logs` | Read process log output |
| `kill` | Send signal to process (default: SIGTERM) |
| `wait` | Block until process exits |
| `delete` | Remove process and logs |
| `daemon` | Manage the daemon (start, stop, status, clean) |

## Example Usage

Start a Python HTTP server in the background:

```console
$ runagent run -n webserver -- python3 -m http.server 9000
✓ Started webserver id=1 pid=3141580
```

Check its status (shows state, resource usage, uptime):

```console
$ runagent ps webserver
webserver #1
State:         Running
PID:           3141580
Command:       python3 -m http.server 9000
Started:       2026-09-18 08:21:37
Uptime:        9s
CPU (3s avg):  0.9%
CPU time:      0.0s user, 0.0s system
RSS:           20 MiB
PSS:           14 MiB
Peak RSS:      20 MiB
Threads:       1
Child procs:   0
Open FDs:      4
Disk I/O:      220 KiB read, 0 B written
Listen:        :9000
```

View its logs (includes periodic resource stats, emitted only when values change):

```console
$ runagent logs webserver
08:21:37.328 │ started python3 -m http.server 9000
08:21:40.168 ~ RSS=20 MiB  PSS=14 MiB  Threads=1  Child procs=0  Open FDs=4  Disk I/O=220 KiB read, 0 B written  Listen=:9000
08:21:54.515   127.0.0.1 - - [18/Sep/2026 08:21:54] "GET / HTTP/1.1" 200 -
08:21:54.533   127.0.0.1 - - [18/Sep/2026 08:21:54] code 404, message File not found
08:21:54.533   127.0.0.1 - - [18/Sep/2026 08:21:54] "GET /index.html HTTP/1.1" 404 -
```

Log line markers: `│` = runagent control messages, `~` = resource stats, blank = process stdout/stderr. Stderr lines are shown in red when color is enabled. Timestamp format is customizable with `--time-format`.

For more details use `--json` to see the full machine readable log records.

Send SIGHUP to the process:

```console
$ runagent kill --signal SIGHUP webserver
✓ Sent SIGHUP to webserver
```

Confirm it was terminated by the signal:

```console
$ runagent ps webserver
webserver #1
State:         Killed
PID:           -
Command:       python3 -m http.server 9000
Started:       2026-09-18 08:21:37
Exited:        2026-09-18 08:22:04
Runtime:       26s
Signal:        SIGHUP(1)
RSS:           20 MiB
PSS:           15 MiB
Peak RSS:      20 MiB
Threads:       1
Child procs:   0
Open FDs:      4
Disk I/O:      304 KiB read, 0 B written
```

Note that `/proc/<pid>/` is unavailable after the process exits so data shown is last recorded historical stats.

To see all processes started by `runagent`:

```console
$ runagent ps
ID  NAME       PID  STATE   COMMAND                      EXIT       UPTIME
──  ─────────  ───  ──────  ───────────────────────────  ─────────  ──────
1   webserver  -    Killed  python3 -m http.server 9000  SIGHUP(1)  26s
```

Re-running a process with the same name auto-replaces the dead one:

```console
$ runagent run -n webserver -- python3 -m http.server 9000
⟳ Replacing previous webserver (Killed, SIGHUP) - old logs discarded
✓ Started webserver id=2 pid=3142600
```

## Using with LLM Agents

Add this to your prompt:

> Use `runagent` to start, monitor, and stop background processes. Run `runagent --help --json` to learn how to use it.
