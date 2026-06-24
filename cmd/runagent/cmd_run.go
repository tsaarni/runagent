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

	if cli.Output == "json" {
		printJSON(resp)
		return
	}
	var data struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		PID  int    `json:"pid"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	fmt.Printf("%s Started %s %s %s\n", successIcon(), bold(data.Name), dim(fmt.Sprintf("id=%d", data.ID)), dim(fmt.Sprintf("pid=%d", data.PID)))
}
