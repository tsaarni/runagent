// Implements the "run" command that spawns a new background process via the daemon.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tsaarni/runagent"
)

func cmdRun() {
	cmd := cli.Run.Command
	if len(cmd) > 0 && cmd[0] == "--" {
		cmd = cmd[1:]
	}
	cwd := cli.Run.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	resp := sendRecv(&runagent.Request{
		Command: "start",
		Args: mustArgs(runagent.StartArgs{
			Command: cmd, Name: cli.Run.Name, Env: cli.Run.Env, Cwd: cwd,
		}),
	})
	checkResp(resp)

	if cli.JSON {
		printJSON(resp)
		return
	}
	var data struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		PID      int    `json:"pid"`
		Replaced *struct {
			ID       int    `json:"id"`
			State    string `json:"state"`
			ExitCode int    `json:"exit_code"`
			Signal   int    `json:"signal"`
		} `json:"replaced"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	if data.Replaced != nil {
		stateInfo := stateColored(data.Replaced.State)
		if data.Replaced.Signal != 0 {
			stateInfo += ", " + signalNameShort(data.Replaced.Signal)
		} else if data.Replaced.ExitCode != 0 {
			stateInfo += fmt.Sprintf(", code %d", data.Replaced.ExitCode)
		}
		fmt.Printf("⟳ Replacing previous %s (%s) - old logs discarded\n", bold(data.Name), stateInfo)
	}
	fmt.Printf("%s Started %s %s %s\n", successIcon(), bold(data.Name), dim(fmt.Sprintf("id=%d", data.ID)), dim(fmt.Sprintf("pid=%d", data.PID)))
}
