// Implements the "kill" command that sends a signal to a managed process.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/tsaarni/runagent"
)

func cmdKill() {
	resp := sendRecv(&runagent.Request{Command: "kill", Args: mustArgs(runagent.KillArgs{Target: cli.Kill.Target, Signal: cli.Kill.Signal})})
	checkResp(resp)

	if cli.Output == "json" {
		printJSON(resp)
		return
	}
	var data map[string]any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	fmt.Printf("%s Sent %s to %s\n", successIcon(), cli.Kill.Signal, bold(fmt.Sprint(data["name"])))
}
