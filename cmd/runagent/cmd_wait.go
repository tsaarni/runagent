// Implements the "wait" command that blocks until a process exits or a timeout is reached.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tsaarni/runagent"
)

func cmdWait() {
	resp := sendRecv(&runagent.Request{Command: "wait", Args: mustArgs(runagent.WaitArgs{Target: cli.Wait.Target, Timeout: cli.Wait.Timeout})})
	checkResp(resp)

	if cli.Output == "json" {
		printJSON(resp)
		return
	}
	var data struct {
		Name     string `json:"name"`
		ExitCode int    `json:"exit_code"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	icon := successIcon()
	if data.ExitCode != 0 {
		icon = failIcon()
	}
	fmt.Printf("%s %s exited (%s, code %d)\n", icon, bold(data.Name), stateColored(data.State), data.ExitCode)
	if data.ExitCode != 0 {
		os.Exit(data.ExitCode)
	}
}
