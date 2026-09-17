package collector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/otelcol"
)

func TestExampleConfigurations(t *testing.T) {
	t.Setenv("OTEL_REDACTOR_BACKEND", "https://backend.example.test:4318")
	for _, name := range []string{"collector.yaml", "production.yaml", "container.yaml"} {
		t.Run(name, func(t *testing.T) {
			col, err := otelcol.NewCollector(Settings("test", filepath.Join("..", "..", "examples", name)))
			if err != nil {
				t.Fatal(err)
			}
			if err := col.DryRun(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInvalidPoliciesPreventStartup(t *testing.T) {
	base := `receivers:
  otlp:
    protocols:
      http:
        endpoint: 127.0.0.1:0
processors:
  otelredactor:
    POLICY
exporters:
  debug: {}
service:
  telemetry:
    metrics:
      level: none
  pipelines:
    traces:
      receivers: [otlp]
      processors: [otelredactor]
      exporters: [debug]
`
	for _, rule := range []string{`patterns: ['[']`, `delete_keys: []`, `delete_key: [password]`, `patterns: ['x']
    max_depth: 0`} {
		t.Run(rule, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.yaml")
			if err := os.WriteFile(path, []byte(strings.Replace(base, "POLICY", rule, 1)), 0600); err != nil {
				t.Fatal(err)
			}
			col, err := otelcol.NewCollector(Settings("test", path))
			if err != nil {
				t.Fatal(err)
			}
			if err := col.DryRun(context.Background()); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
