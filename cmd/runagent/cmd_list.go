// Implements the "list" command that displays all managed processes in a table.

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tsaarni/runagent"
)

func cmdList() {
	resp := sendRecv(&runagent.Request{Command: "list", Args: mustArgs(struct{}{})})
	checkResp(resp)

	if cli.Output == "json" {
		printJSON(resp)
		return
	}

	var procs []struct {
		ID        int      `json:"id"`
		Name      string   `json:"name"`
		PID       int      `json:"pid"`
		State     string   `json:"state"`
		Command   []string `json:"command"`
		ExitCode  int      `json:"exit_code"`
		Signal    int      `json:"signal"`
		StartedAt string   `json:"started_at"`
		ExitedAt  string   `json:"exited_at"`
	}
	if err := json.Unmarshal(resp.Data, &procs); err != nil {
		fatalf("decode response: %v", err)
	}

	if len(procs) == 0 {
		fmt.Println(dim("No managed processes"))
		return
	}

	t := newTable("ID", "NAME", "PID", "STATE", "COMMAND", "EXIT", "UPTIME")
	for _, p := range procs {
		cmdStr := strings.Join(p.Command, " ")
		if len(cmdStr) > 50 {
			cmdStr = cmdStr[:47] + "..."
		}

		pidStr := "-"
		if p.State == "Running" {
			pidStr = strconv.Itoa(p.PID)
		}

		exitStr := ""
		switch p.State {
		case "Exited":
			exitStr = strconv.Itoa(p.ExitCode)
			if p.ExitCode != 0 {
				exitStr = red(exitStr)
			} else {
				exitStr = dim(exitStr)
			}
		case "Crashed":
			exitStr = red(signalName(p.Signal))
		}

		t.row(
			dim(strconv.Itoa(p.ID)),
			bold(p.Name),
			dim(pidStr),
			stateColored(p.State),
			dim(cmdStr),
			exitStr,
			dim(formatDuration(p.StartedAt, p.ExitedAt)),
		)
	}
	t.print()
}
