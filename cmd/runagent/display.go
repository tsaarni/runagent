// Provides terminal formatting utilities: ANSI colors, table rendering, and key-value display.

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var noColor = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	fi, _ := os.Stdout.Stat()
	return fi.Mode()&os.ModeCharDevice == 0
}()

func ansi(code, s string) string {
	if noColor || s == "" {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func bold(s string) string   { return ansi("1", s) }
func dim(s string) string    { return ansi("2", s) }
func red(s string) string    { return ansi("31", s) }
func green(s string) string  { return ansi("32", s) }
func yellow(s string) string { return ansi("33", s) }
func white(s string) string  { return ansi("37", s) }
func boldRed(s string) string { return ansi("1;31", s) }
func dimCyan(s string) string { return ansi("36", s) }

func successIcon() string { return green("✓") }
func failIcon() string    { return red("✗") }

func stateColored(s string) string {
	switch s {
	case "Running":
		return green(s)
	case "Exited":
		return white(s)
	case "Crashed":
		return boldRed(s)
	default:
		return s
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visibleLen(s string) int {
	return len(ansiRe.ReplaceAllString(s, ""))
}

func pad(s string, width int) string {
	gap := width - visibleLen(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// --- Table ---

type table struct {
	headers []string
	rows    [][]string
}

func newTable(headers ...string) *table {
	return &table{headers: headers}
}

func (t *table) row(values ...string) {
	t.rows = append(t.rows, values)
}

func (t *table) print() {
	ncols := len(t.headers)
	widths := make([]int, ncols)
	for i, h := range t.headers {
		widths[i] = len(h)
	}
	for _, row := range t.rows {
		for i := 0; i < ncols && i < len(row); i++ {
			if vl := visibleLen(row[i]); vl > widths[i] {
				widths[i] = vl
			}
		}
	}

	// Header row - bold dim for understated authority
	parts := make([]string, ncols)
	for i, h := range t.headers {
		parts[i] = pad(dim(bold(h)), widths[i])
	}
	fmt.Println(strings.Join(parts, "  "))

	// Separator line
	sepParts := make([]string, ncols)
	for i := range t.headers {
		sepParts[i] = dim(strings.Repeat("─", widths[i]))
	}
	fmt.Println(strings.Join(sepParts, "  "))

	// Data rows
	for _, row := range t.rows {
		parts := make([]string, ncols)
		for i := 0; i < ncols; i++ {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			parts[i] = pad(val, widths[i])
		}
		fmt.Println(strings.Join(parts, "  "))
	}
}

// --- Props ---

type props struct {
	pairs      []prop
	labelWidth int
}

type prop struct {
	label string
	value string
}

func newProps() *props {
	return &props{}
}

func (p *props) add(label, value string) {
	p.pairs = append(p.pairs, prop{label, value})
}

func (p *props) print() {
	if len(p.pairs) == 0 {
		return
	}
	w := p.labelWidth
	if w == 0 {
		for _, pair := range p.pairs {
			if len(pair.label) > w {
				w = len(pair.label)
			}
		}
	}
	for _, pair := range p.pairs {
		fmt.Printf("%s  %s\n", pad(dim(pair.label), w), pair.value)
	}
}

var signalNames = map[int]string{
	1: "HUP", 2: "INT", 3: "QUIT", 4: "ILL", 5: "TRAP", 6: "ABRT",
	7: "BUS", 8: "FPE", 9: "KILL", 10: "USR1", 11: "SEGV", 12: "USR2",
	13: "PIPE", 14: "ALRM", 15: "TERM",
}

func signalName(num int) string {
	if name, ok := signalNames[num]; ok {
		return fmt.Sprintf("sig:%d(%s)", num, name)
	}
	return fmt.Sprintf("sig:%d", num)
}
