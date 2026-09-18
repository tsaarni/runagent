// Implements the "delete" command that removes a process entry and its logs.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/tsaarni/runagent"
)

func cmdDelete() {
	da := runagent.DeleteArgs{All: cli.Delete.All, Target: cli.Delete.Target}
	if !da.All && da.Target == "" {
		fatalf("specify a target or --all")
	}

	resp := sendRecv(&runagent.Request{Command: "delete", Args: mustArgs(da)})
	checkResp(resp)

	if cli.JSON {
		printJSON(resp)
		return
	}
	var data map[string]any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}
	if n, ok := data["deleted"]; ok {
		fmt.Printf("%s Deleted %.0f processes\n", successIcon(), n)
	} else {
		fmt.Printf("%s Deleted %s\n", successIcon(), bold(fmt.Sprint(data["name"])))
	}
}
