// Implements the "delete" command that kills a process (if running) and deletes its logs.
// With --stopped, only processes that are not running are deleted.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/tsaarni/runagent"
)

func cmdDelete() {
	da := runagent.DeleteArgs{All: cli.Delete.All, Target: cli.Delete.Target}
	if !da.All && !cli.Delete.Stopped && da.Target == "" {
		fatalf("specify a target, --all or --stopped")
	}

	var resp *runagent.Response
	if cli.Delete.Stopped {
		resp = deleteStopped()
	} else {
		resp = sendRecv(&runagent.Request{Command: "delete", Args: mustArgs(da)})
	}
	checkResp(resp)

	if cli.JSON {
		printJSON(resp)
		return
	}
	var data map[string]any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	if names, ok := data["names"].([]any); ok {
		if len(names) == 0 {
			fmt.Println(dim("No stopped processes"))
			return
		}
		for _, name := range names {
			fmt.Printf("%s Deleted %s\n", successIcon(), bold(fmt.Sprint(name)))
		}
		fmt.Printf("%s Deleted %d processes\n", successIcon(), len(names))
	} else if n, ok := data["deleted"]; ok {
		fmt.Printf("%s Deleted %.0f processes\n", successIcon(), n)
	} else {
		fmt.Printf("%s Deleted %s\n", successIcon(), bold(fmt.Sprint(data["name"])))
	}
}

// deleteStopped lists all processes and deletes each one whose state is not Running
// (Exited or Killed). Returns a response with the number and names of deleted processes.
func deleteStopped() *runagent.Response {
	resp := sendRecv(&runagent.Request{Command: "status", Args: mustArgs(runagent.StatusArgs{})})
	checkResp(resp)

	var procs []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(resp.Data, &procs); err != nil {
		fatalf("decode response: %v", err)
	}

	names := []string{}
	for _, p := range procs {
		if p.State == "Running" {
			continue
		}
		checkResp(sendRecv(&runagent.Request{Command: "delete", Args: mustArgs(runagent.DeleteArgs{Target: p.Name})}))
		names = append(names, p.Name)
	}
	return &runagent.Response{OK: true, Data: mustArgs(map[string]any{"deleted": len(names), "names": names})}
}
