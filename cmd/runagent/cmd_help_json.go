// Generates machine-readable JSON help output describing all CLI commands and flags.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

func printHelpJSON() {
	app, err := kong.New(&cli)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var cmds []compactCmd
	for _, node := range app.Model.Children {
		if node.Hidden {
			continue
		}
		if len(node.Children) > 0 {
			for _, child := range node.Children {
				if child.Hidden {
					continue
				}
				cmds = append(cmds, buildCmd(node.Name+" "+child.Name, child))
			}
		} else {
			cmds = append(cmds, buildCmd(node.Name, node))
		}
	}

	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(cmds)
}

type compactCmd struct {
	Cmd   string   `json:"cmd"`
	Desc  string   `json:"desc"`
	Flags []string `json:"flags,omitempty"`
}

func buildCmd(name string, node *kong.Node) compactCmd {
	cmd := "runagent " + name
	for _, p := range node.Positional {
		if p.Required {
			cmd += " <" + p.Name + ">"
		} else {
			cmd += " [" + p.Name + "]"
		}
	}

	c := compactCmd{
		Cmd:  cmd,
		Desc: node.Help,
	}

	for _, f := range node.Flags {
		if f.Hidden || f.Name == "help" {
			continue
		}
		s := "--" + f.Name
		if f.HasDefault && f.Default != "" && f.Default != "false" && f.Default != "0" {
			s += "=" + f.Default
		}
		s += ": " + f.Help
		c.Flags = append(c.Flags, s)
	}
	return c
}
