package demo

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Garvity/otel-redactor/internal/collector"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Exercise the operational pipeline, including real TLS certificate validation,
// batching, queue/retry, all advertised ingress transports, and live metrics.
func TestOperationalTransportsAndMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	received := make(chan []byte, 8)
	var attempts atomic.Int32
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.URL.Path != "/v1/traces" {
			http.NotFound(w, r)
			return
		}
		if attempts.Add(1) == 1 {
			http.Error(w, "synthetic transient failure", http.StatusServiceUnavailable)
			return
		}
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer gz.Close()
			reader = gz
		}
		body, err := io.ReadAll(io.LimitReader(reader, 4<<20))
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		td, err := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		data, err := (&ptrace.JSONMarshaler{}).MarshalTraces(td)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		select {
		case received <- data:
		case <-ctx.Done():
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sink.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	httpAddr, grpcAddr, metricsAddr := freeAddress(t), freeAddress(t), freeAddress(t)
	_, metricsPort, err := net.SplitHostPort(metricsAddr)
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile("../../examples/production.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OTEL_REDACTOR_BACKEND", sink.URL)
	s := strings.Replace(string(config), "127.0.0.1:4318", httpAddr, 1)
	s = strings.Replace(s, "port: 8888", "port: "+metricsPort, 1)
	s = strings.Replace(s, "protocols:\n", "protocols:\n      grpc:\n        endpoint: "+grpcAddr+"\n", 1)
	s = strings.Replace(s, "  otelredactor:\n", "  otelredactor:\n    max_string_bytes: 64\n", 1)
	s = strings.Replace(s, "    timeout: 1s", "    timeout: 50ms", 1)
	s = strings.Replace(s, "    timeout: 10s", fmt.Sprintf("    timeout: 2s\n    tls:\n      ca_file: %s", ca), 1)
	s = strings.Replace(s, "      max_elapsed_time: 60s", "      initial_interval: 50ms\n      max_interval: 100ms\n      max_elapsed_time: 2s", 1)
	stop := launchCollector(t, ctx, dir, s)
	defer stop()
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	post := func(body []byte, contentType string) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, "POST", "http://"+httpAddr+"/v1/traces", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}
	verifyExport := func() {
		t.Helper()
		select {
		case body := <-received:
			if err := Verify(body); err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	jsonInput, err := (&ptrace.JSONMarshaler{}).MarshalTraces(Fixture(false))
	if err != nil {
		t.Fatal(err)
	}
	if code := post(jsonInput, "application/json"); code != 200 {
		t.Fatalf("JSON ingress: HTTP %d", code)
	}
	verifyExport()
	protoInput, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(Fixture(false))
	if err != nil {
		t.Fatal(err)
	}
	if code := post(protoInput, "application/x-protobuf"); code != 200 {
		t.Fatalf("protobuf ingress: HTTP %d", code)
	}
	verifyExport()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	grpcClient := ptraceotlp.NewGRPCClient(conn)
	if _, err := grpcClient.Export(ctx, ptraceotlp.NewExportRequestFromTraces(Fixture(false))); err != nil {
		t.Fatal(err)
	}
	verifyExport()
	if attempts.Load() != 4 {
		t.Fatalf("expected 3 deliveries + 1 retry, got %d attempts", attempts.Load())
	}
	bad := Fixture(false)
	bad.ResourceSpans().At(0).Resource().Attributes().PutStr("oversize", strings.Repeat("x", 65))
	if _, err := grpcClient.Export(ctx, ptraceotlp.NewExportRequestFromTraces(bad)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("gRPC rejection: %v", err)
	}
	badJSON, err := (&ptrace.JSONMarshaler{}).MarshalTraces(bad)
	if err != nil {
		t.Fatal(err)
	}
	if code := post(badJSON, "application/json"); code != 400 {
		t.Fatalf("HTTP rejection: %d", code)
	}
	resp, err := client.Get("http://" + metricsAddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	metrics, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("metrics HTTP %d", resp.StatusCode)
	}
	for name, want := range map[string]int{"attributes_removed": 18, "values_redacted": 24, "batches_rejected": 2} {
		pattern := fmt.Sprintf(`(?m)^otelredactor_%s(?:_[a-z]+)*(?:\{[^\n]*\})? %d(?:\.0)?$`, name, want)
		if !regexp.MustCompile(pattern).Match(metrics) {
			t.Errorf("live counter %s missing or incorrect; want %d; scrape:\n%s", name, want, metrics)
		}
	}
	if bytes.Contains(metrics, []byte("alice@example.test")) || bytes.Contains(metrics, []byte("synthetic-secret-do-not-use")) {
		t.Error("metrics contain payload values")
	}
	stop()
	select {
	case <-received:
		t.Fatal("rejected batch was exported")
	default:
	}
}

func TestShutdownFlushesBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	received := make(chan []byte, 1)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		select {
		case received <- data:
		default:
			t.Error("unexpected extra export")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer sink.Close()
	addr := freeAddress(t)
	config := fmt.Sprintf(`receivers:
  otlp:
    protocols:
      http:
        endpoint: %s
processors:
  otelredactor:
    delete_keys: [authorization, api.key, password]
    patterns: ['(?i)[a-z0-9._%%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}']
  batch:
    timeout: 1h
    send_batch_size: 512
exporters:
  otlp_http:
    endpoint: %s
    encoding: json
    compression: none
    sending_queue:
      enabled: false
service:
  telemetry:
    logs:
      level: error
    metrics:
      level: none
  pipelines:
    traces:
      receivers: [otlp]
      processors: [otelredactor, batch]
      exporters: [otlp_http]
`, addr, sink.URL)
	stop := launchCollector(t, ctx, t.TempDir(), config)
	defer stop()
	data, err := (&ptrace.JSONMarshaler{}).MarshalTraces(Fixture(false))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Post("http://"+addr+"/v1/traces", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	select {
	case <-received:
		t.Fatal("batch exported before flush")
	default:
	}
	stop()
	select {
	case body := <-received:
		if err := Verify(body); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("shutdown lost buffered trace")
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func launchCollector(t *testing.T, parent context.Context, dir, config string) func() {
	t.Helper()
	path := filepath.Join(dir, "runtime.yaml")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	set := collector.Settings("runtime-test", path)
	set.DisableGracefulShutdown = true
	set.SkipSettingGRPCLogger = true
	col, err := otelcol.NewCollector(set)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("shutdown timeout")
		}
	}
	t.Cleanup(stop)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			done <- err
			t.Fatalf("startup failed: %v", err)
		case <-parent.Done():
			t.Fatal(parent.Err())
		case <-ticker.C:
			if col.GetState() == otelcol.StateRunning {
				return stop
			}
		}
	}
}
