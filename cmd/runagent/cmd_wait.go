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

	if cli.JSON {
		printJSON(resp)
		return
	}
	var data struct {
		Name     string `json:"name"`
		ExitCode int    `json:"exit_code"`
		Signal   int    `json:"signal"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}

	exitCode := data.ExitCode
	if data.Signal != 0 {
		exitCode = 128 + data.Signal
	}

	icon := successIcon()
	if exitCode != 0 {
		icon = failIcon()
	}

	if data.Signal != 0 {
		fmt.Printf("%s %s exited (%s, signal %s, code %d)\n", icon, bold(data.Name), stateColored(data.State), signalNameShort(data.Signal), exitCode)
	} else {
		fmt.Printf("%s %s exited (%s, code %d)\n", icon, bold(data.Name), stateColored(data.State), exitCode)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
