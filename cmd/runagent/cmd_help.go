// Generates CLI help output describing all CLI commands and flags.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
)

type helpWriter struct {
	w   io.Writer
	err error
}

func (hw *helpWriter) printf(format string, args ...any) {
	if hw.err != nil {
		return
	}
	_, hw.err = fmt.Fprintf(hw.w, format, args...)
}

func (hw *helpWriter) println(args ...any) {
	if hw.err != nil {
		return
	}
	_, hw.err = fmt.Fprintln(hw.w, args...)
}

func printFullHelp(w io.Writer, app *kong.Application) error {
	hw := &helpWriter{w: w}

	hw.printf("Usage: %s <command> [flags]\n\n", app.Name)

	hw.println("Flags:")
	hw.println("  -h, --help    Show context-sensitive help.")
	hw.println("      --json    Output in JSON format.")
	hw.println()

	hw.println("Commands:")
	for _, node := range app.Children {
		if node.Hidden {
			continue
		}
		if len(node.Children) > 0 {
			for _, child := range node.Children {
				if child.Hidden {
					continue
				}
				hw.printf("  %s %s\n    %s\n\n", node.Name, child.Name, child.Help)
			}
		} else {
			hw.printf("  %s\n    %s\n", cmdUsage(node), node.Help)
			rows := formatNodeFlags(node)
			const flagColWidth = 25
			for _, r := range rows {
				if len(r[0]) >= flagColWidth {
					hw.printf("    %s  %s\n", r[0], r[1])
				} else {
					hw.printf("    %-*s  %s\n", flagColWidth, r[0], r[1])
				}
			}
			hw.println()
		}
	}

	hw.println("Examples:")
	hw.println("  runagent run -n web -- python3 -m http.server 8080")
	hw.println("  runagent ps")
	hw.println("  runagent logs web --last 20")
	hw.println("  runagent wait web")
	hw.println("  runagent delete web")

	return hw.err
}

func formatNodeFlags(node *kong.Node) [][2]string {
	var rows [][2]string
	for _, f := range node.Flags {
		if f.Hidden || f.Name == "help" {
			continue
		}
		var flagStr string
		if f.Short != 0 {
			flagStr = fmt.Sprintf("-%c, --%s", f.Short, f.Name)
		} else {
			flagStr = fmt.Sprintf("    --%s", f.Name)
		}

		if !f.IsBool() {
			placeholder := f.FormatPlaceHolder()
			flagStr += "=" + placeholder
		}
		rows = append(rows, [2]string{flagStr, f.Help})
	}
	return rows
}

func cmdUsage(node *kong.Node) string {
	cmd := node.Name
	for _, p := range node.Positional {
		if p.Required {
			cmd += " <" + p.Name + ">"
		} else {
			cmd += " [" + p.Name + "]"
		}
	}
	return cmd
}

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
	enc.SetEscapeHTML(false)
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
