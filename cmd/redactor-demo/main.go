package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Garvity/otel-redactor/internal/demo"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo failed:", err)
		os.Exit(1)
	}
}

func run() error {
	output := flag.String("output", "demo-output", "directory for synthetic before/after JSON and verified report")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result, err := demo.Run(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	for name, value := range map[string]any{"before.json": result.Before, "after.json": result.After, "report.json": result} {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*output, name), append(data, '\n'), 0600); err != nil {
			return err
		}
	}
	fmt.Println("otel-redactor | verified synthetic OTLP round trip")
	for _, check := range result.Checks {
		fmt.Println("PASS", check)
	}
	fmt.Println("PASS Collector shut down cleanly")
	fmt.Println("Evidence:", *output)
	return nil
}
