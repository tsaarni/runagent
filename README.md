# runagent

Background process manager for agents.

## Description

When using LLM agent to debug and troubleshoot applications, it typically uses `myapp &` and then `ps | grep` to check the process.
This approach is fragile:

- The agent might lose track of the process or find the wrong one
- If the process exits with an error, the agent may not notice and continues trying to interact with it
- The agent might not collect stdout/stderr output
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
| `ps` | List all managed processes |
| `status` | Show detailed process metrics |
| `logs` | Read process log output |
| `kill` | Send signal to process (default: SIGTERM) |
| `wait` | Block until process exits |
| `delete` | Remove process and logs |
| `shutdown` | Gracefully stop daemon and all processes |
| `daemon` | Manage the daemon (start, stop, status, clean) |

## Example Usage

Start a Python HTTP server in the background:

```console
$ runagent run -n webserver -- python3 -m http.server 9000
✓ Started webserver id=1 pid=1398124
```

Check its status (shows state, resource usage, uptime):

```console
$ runagent status webserver
webserver #1
State:         Running
PID:           1398124
Command:       python3 -m http.server 9000
Started:       2026-06-26 08:25:47
Uptime:        20s
CPU (3s avg):  0.0%
CPU time:      0.0s user, 0.0s system
RSS:           20 MiB
PSS:           16 MiB
Peak RSS:      20 MiB
Threads:       1
Child procs:   0
Open FDs:      7
Disk I/O:      4.0 KiB read, 0 B written
```

View its logs (includes periodic resource stats, emitted only when values change):

```console
$ runagent logs webserver
08:25:47.388 │ started python3 -m http.server 9000
08:25:48.366 ~ RSS=20 MiB  PSS=16 MiB  Threads=1  Child procs=0  Open FDs=7  Disk I/O=4.0 KiB read, 0 B written
08:26:05.642   127.0.0.1 - - [26/Jun/2026 08:26:05] "GET / HTTP/1.1" 200 -
08:26:05.653   127.0.0.1 - - [26/Jun/2026 08:26:05] code 404, message File not found
08:26:05.653   127.0.0.1 - - [26/Jun/2026 08:26:05] "GET /index.html HTTP/1.1" 404 -
```

Log line markers: `│` = runagent control messages, `~` = resource stats, blank = process stdout/stderr. Stderr lines are shown in red when color is enabled. Timestamp format is customizable with `--time-format`.

For more details use `--json`  to see the full machine readable log records.

Send SIGHUP to the process:

```console
$ runagent kill --signal SIGHUP webserver
✓ Sent SIGHUP to webserver
```

Confirm it was terminated by the signal:

```console
$ runagent status webserver
webserver #1
State:         Crashed
PID:           -
Command:       python3 -m http.server 9000
Started:       2026-06-26 08:25:47
Exited:        2026-06-26 08:26:13
Runtime:       25s
Signal:        sig:1(HUP)
RSS:           20 MiB
PSS:           16 MiB
Threads:       1
Child procs:   0
Open FDs:      7
Disk I/O:      4.0 KiB read, 0 B written
```

Note that `/proc/<pid>/` is unavailable after the process exits so data shown is last recorded historical stats.

To see all processes started by `runagent`:

```console
$ runagent ps
ID  NAME       PID  STATE    COMMAND                      EXIT        UPTIME
──  ─────────  ───  ───────  ───────────────────────────  ──────────  ──────
1   webserver  -    Crashed  python3 -m http.server 9000  sig:1(HUP)  25s
```

## Using with LLM Agents

Add this to your prompt:

> Use `runagent` to start, monitor, and stop background processes. Run `runagent --help --output=json` to learn how to use it.
