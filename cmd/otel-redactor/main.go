package main

import (
	"os"

	"github.com/Garvity/otel-redactor/internal/collector"
	"go.opentelemetry.io/collector/otelcol"
)

var version = "dev"

func main() {
	command := otelcol.NewCommand(collector.Settings(version))
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
