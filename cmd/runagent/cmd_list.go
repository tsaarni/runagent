// Implements the "ps" command: table overview (no target) or detailed view (with target).

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tsaarni/runagent"
)

func cmdPs() {
	resp := sendRecv(&runagent.Request{Command: "status", Args: mustArgs(runagent.StatusArgs{Target: cli.Ps.Target})})
	checkResp(resp)

	if cli.JSON {
		out := map[string]any{"ok": true, "error": "", "data": json.RawMessage(resp.Data)}
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	var procs []map[string]any
	if err := json.Unmarshal(resp.Data, &procs); err != nil {
		fatalf("decode response: %v", err)
	}

	if len(procs) == 0 {
		fmt.Println(dim("No managed processes"))
		return
	}

	// Detailed view: single target or --all
	if cli.Ps.Target != "" || cli.Ps.All {
		for i, data := range procs {
			if i > 0 {
				fmt.Println()
			}
			printStatusProps(data)
		}
		return
	}

	// Table view
	t := newTable("ID", "NAME", "PID", "STATE", "COMMAND", "EXIT", "UPTIME")
	for _, p := range procs {
		id := int(p["id"].(float64))
		name := fmt.Sprint(p["name"])
		state := fmt.Sprint(p["state"])
		cmdSlice := toStringSlice(p["command"])
		cmdStr := strings.Join(cmdSlice, " ")
		if len(cmdStr) > 50 {
			cmdStr = cmdStr[:47] + "..."
		}

		pidStr := "-"
		if state == "Running" {
			pidStr = fmt.Sprintf("%.0f", p["pid"].(float64))
		}

		exitStr := ""
		switch state {
		case "Exited":
			ec := int(p["exit_code"].(float64))
			exitStr = strconv.Itoa(ec)
			if ec != 0 {
				exitStr = red(exitStr)
			} else {
				exitStr = dim(exitStr)
			}
		case "Killed":
			sig := int(p["signal"].(float64))
			exitStr = red(signalName(sig))
		}

		sa, _ := p["started_at"].(string)
		ea, _ := p["exited_at"].(string)

		t.row(
			dim(strconv.Itoa(id)),
			bold(name),
			dim(pidStr),
			stateColored(state),
			dim(cmdStr),
			exitStr,
			dim(formatDuration(sa, ea)),
		)
	}
	t.print()
}

func printStatusProps(data map[string]any) {
	state := fmt.Sprint(data["state"])
	cmdSlice := toStringSlice(data["command"])
	sa, _ := data["started_at"].(string)
	ea, _ := data["exited_at"].(string)

	// Section header
	fmt.Printf("%s %s\n", bold(fmt.Sprint(data["name"])), dim(fmt.Sprintf("#%v", data["id"])))

	kv := newProps()
	kv.labelWidth = len("CPU (3s avg):")
	kv.add("State:", stateColored(state))
	pidStr := "-"
	if state == "Running" {
		pidStr = fmt.Sprintf("%.0f", data["pid"].(float64))
	}
	kv.add("PID:", pidStr)
	kv.add("Command:", strings.Join(cmdSlice, " "))
	if sa != "" {
		kv.add("Started:", formatTime(sa))
		switch state {
		case "Running":
			kv.add("Uptime:", formatDuration(sa, ""))
		default:
			if ea != "" {
				kv.add("Exited:", formatTime(ea))
				kv.add("Runtime:", formatDuration(sa, ea))
			}
		}
	}
	if state == "Exited" {
		ec := int(data["exit_code"].(float64))
		code := strconv.Itoa(ec)
		if ec != 0 {
			code = red(code)
		} else {
			code = dim(code)
		}
		kv.add("Exit code:", code)
	}
	if state == "Killed" {
		sig := int(data["signal"].(float64))
		if sig != 0 {
			kv.add("Signal:", red(signalName(sig)))
		}
	}

	if stats, ok := data["stats"].([]any); ok {
		for _, s := range stats {
			m := s.(map[string]any)
			kv.add(m["label"].(string)+":", m["value"].(string))
		}
	}
	kv.print()
}
