// Implements the "status" command that shows detailed process metrics and resource usage.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tsaarni/runagent"
)

func cmdStatus() {
	if cli.Status.Target == "" {
		cmdStatusAll()
		return
	}

	resp := sendRecv(&runagent.Request{Command: "status", Args: mustArgs(runagent.StatusArgs{Target: cli.Status.Target})})
	checkResp(resp)

	if cli.Output == "json" {
		printJSON(resp)
		return
	}

	var data map[string]any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	printStatusProps(data)
}

func cmdStatusAll() {
	resp := sendRecv(&runagent.Request{Command: "list", Args: mustArgs(struct{}{})})
	checkResp(resp)

	var procs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Data, &procs); err != nil {
		fatalf("decode response: %v", err)
	}

	if len(procs) == 0 {
		fmt.Println(dim("No managed processes"))
		return
	}

	if cli.Output == "json" {
		var results []json.RawMessage
		for _, p := range procs {
			r := sendRecv(&runagent.Request{Command: "status", Args: mustArgs(runagent.StatusArgs{Target: p.Name})})
			if r.OK {
				results = append(results, r.Data)
			}
		}
		out := map[string]any{"ok": true, "error": "", "data": results}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	for i, p := range procs {
		r := sendRecv(&runagent.Request{Command: "status", Args: mustArgs(runagent.StatusArgs{Target: p.Name})})
		if !r.OK {
			continue
		}
		if i > 0 {
			fmt.Println()
		}
		var data map[string]any
		if err := json.Unmarshal(r.Data, &data); err != nil {
			continue
		}
		printStatusProps(data)
	}
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
	if state == "Crashed" {
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
