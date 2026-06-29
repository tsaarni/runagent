// Implements the "logs" command that reads and displays process log output with filtering.

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tsaarni/runagent"
)

func cmdLogs() {
	resp := sendRecv(&runagent.Request{Command: "logs", Args: mustArgs(runagent.LogsArgs{Target: cli.Logs.Target})})
	checkResp(resp)

	var data struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fatalf("decode response: %v", err)
	}

	// Build filter
	types := make(map[string]bool)
	for _, t := range strings.Split(cli.Logs.Type, ",") {
		types[strings.TrimSpace(t)] = true
	}

	var tr TimeRange
	if cli.Logs.TimeRange != "" {
		var err error
		tr, err = parseTimeRange(cli.Logs.TimeRange, time.Now())
		if err != nil {
			fatalf("%v", err)
		}
	}

	if cli.Logs.Limit > 0 && cli.Logs.Last > 0 {
		fatalf("--limit and --last are mutually exclusive")
	}

	filter := logFilter{
		types:     types,
		stream:    cli.Logs.Stream,
		timeRange: tr,
		limit:     cli.Logs.Limit,
		last:      cli.Logs.Last,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	if err := readLog(ctx, data.Path, filter, cli.Logs.Follow, cli.Output == "json", resolveTimeFormat(cli.Logs.TimeFormat), os.Stdout); err != nil && err != context.Canceled {
		fatalf("%v", err)
	}
}
