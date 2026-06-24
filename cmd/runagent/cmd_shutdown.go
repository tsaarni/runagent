// Implements the "shutdown" command that gracefully stops the daemon and all processes.

package main

import (
	"fmt"

	"github.com/tsaarni/runagent"
)

func cmdShutdown() {
	resp := sendRecv(&runagent.Request{Command: "shutdown", Args: mustArgs(struct{}{})})
	checkResp(resp)

	if cli.Output == "json" {
		printJSON(resp)
		return
	}
	fmt.Printf("%s Daemon shutting down\n", successIcon())
}
