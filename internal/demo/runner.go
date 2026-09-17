package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Garvity/otel-redactor/internal/collector"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Result contains only synthetic fixture data and verified statements.
type Result struct {
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
	Checks []string        `json:"checks"`
}

// Run exercises actual OTLP HTTP ingress and egress through a running Collector.
func Run(ctx context.Context) (result Result, err error) {
	h, err := start(ctx, 1<<20)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, h.close()) }()
	before, err := (&ptrace.JSONMarshaler{}).MarshalTraces(Fixture(false))
	if err != nil {
		return result, err
	}
	status, response, err := h.post(ctx, before)
	if err != nil {
		return result, err
	}
	if status != http.StatusOK {
		return result, fmt.Errorf("OTLP receiver returned HTTP %d", status)
	}
	if strings.Contains(string(response), "rejectedSpans") {
		return result, errors.New("OTLP receiver partially rejected the fixture")
	}
	var after []byte
	select {
	case after = <-h.received:
	case <-ctx.Done():
		return result, ctx.Err()
	}
	if err := Verify(after); err != nil {
		return result, err
	}
	return Result{Before: before, After: after, Checks: []string{
		"OTLP HTTP receiver -> otelredactor -> OTLP HTTP exporter completed",
		"6 configured secret attributes removed, including nested password",
		"8 matching strings redacted across attributes and text fields",
		"Resource, scope, span, event, link, and nested array verified",
		"2 spans preserved with original IDs, parent relationship, and timestamps",
		"Numeric, boolean, and unrelated string values preserved",
		"Complete exported trace matches the independently constructed expectation",
	}}, nil
}

// Verify compares the complete exported trace against an independently built
// expected fixture, ignoring only attribute-map ordering in OTLP JSON.
func Verify(after []byte) error {
	if bytes.Contains(after, []byte("alice@example.test")) || bytes.Contains(after, []byte("synthetic-secret-do-not-use")) {
		return errors.New("synthetic sensitive value survived redaction")
	}
	want, err := (&ptrace.JSONMarshaler{}).MarshalTraces(Fixture(true))
	if err != nil {
		return err
	}
	// Round-trip through pdata to normalize exporter default field serialization.
	td, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(after)
	if err != nil {
		return fmt.Errorf("decode exported trace: %w", err)
	}
	got, err := (&ptrace.JSONMarshaler{}).MarshalTraces(td)
	if err != nil {
		return err
	}
	g, err := canonical(got)
	if err != nil {
		return err
	}
	w, err := canonical(want)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(g, w) {
		return errors.New("exported trace differs from expected redaction or preservation behavior")
	}
	return nil
}

func canonical(data []byte) (any, error) {
	var v any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return nil, err
	}
	var walk func(any)
	walk = func(value any) {
		switch x := value.(type) {
		case map[string]any:
			for key, child := range x {
				if key == "attributes" || key == "values" {
					if list, ok := child.([]any); ok && isKeyValueList(list) {
						sort.Slice(list, func(i, j int) bool {
							return list[i].(map[string]any)["key"].(string) < list[j].(map[string]any)["key"].(string)
						})
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(v)
	return v, nil
}

func isKeyValueList(list []any) bool {
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return false
		}
		if _, ok := m["key"].(string); !ok {
			return false
		}
	}
	return true
}

type harness struct {
	endpoint string
	received chan []byte
	sink     *httptest.Server
	cancel   context.CancelFunc
	done     chan error
	dir      string
	once     sync.Once
	closeErr error
}

func start(parent context.Context, maxBytes int) (*harness, error) {
	h := &harness{received: make(chan []byte, 64), done: make(chan error, 1)}
	// Bind only loopback and explicitly bound server request sizes/timeouts.
	h.sink = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			http.NotFound(w, r)
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		select {
		case h.received <- body:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		default:
			http.Error(w, "demo sink full", http.StatusServiceUnavailable)
		}
	}))
	h.sink.Config.ReadHeaderTimeout = 5 * time.Second
	h.sink.Config.ReadTimeout = 5 * time.Second
	h.sink.Config.WriteTimeout = 5 * time.Second
	h.sink.Start()
	var err error
	h.dir, err = os.MkdirTemp("", "otel-redactor-demo-")
	if err != nil {
		h.sink.Close()
		return nil, err
	}
	// Reserve then release a port for the Collector, which owns its listener.
	// If another process claims it in between, startup returns an explicit error.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		h.sink.Close()
		_ = os.RemoveAll(h.dir)
		return nil, err
	}
	address := l.Addr().String()
	if err := l.Close(); err != nil {
		h.sink.Close()
		_ = os.RemoveAll(h.dir)
		return nil, err
	}
	h.endpoint = "http://" + address + "/v1/traces"
	config := fmt.Sprintf(`receivers:
  otlp:
    protocols:
      http:
        endpoint: %s
        max_request_body_size: 4194304
processors:
  otelredactor:
    delete_keys: [authorization, api.key, password]
    patterns: ['(?i)[a-z0-9._%%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}']
    max_string_bytes: %d
exporters:
  otlp_http:
    endpoint: %s
    encoding: json
    compression: none
    sending_queue:
      enabled: false
    retry_on_failure:
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
      processors: [otelredactor]
      exporters: [otlp_http]
`, address, maxBytes, h.sink.URL)
	path := filepath.Join(h.dir, "collector.yaml")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		h.sink.Close()
		_ = os.RemoveAll(h.dir)
		return nil, err
	}
	set := collector.Settings("demo", path)
	set.DisableGracefulShutdown = true
	set.SkipSettingGRPCLogger = true
	col, err := otelcol.NewCollector(set)
	if err != nil {
		h.sink.Close()
		_ = os.RemoveAll(h.dir)
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	h.cancel = cancel
	go func() { h.done <- col.Run(ctx) }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-h.done:
			h.done <- err
			_ = h.close()
			return nil, fmt.Errorf("collector exited before readiness: %w", err)
		case <-timer.C:
			_ = h.close()
			return nil, errors.New("collector readiness timed out")
		case <-ctx.Done():
			_ = h.close()
			return nil, ctx.Err()
		case <-tick.C:
			if col.GetState() == otelcol.StateRunning {
				return h, nil
			}
		}
	}
}

func (h *harness) post(ctx context.Context, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, err
}

func (h *harness) close() error {
	h.once.Do(func() {
		h.cancel()
		select {
		case err := <-h.done:
			h.closeErr = err
		case <-time.After(5 * time.Second):
			h.closeErr = errors.New("collector shutdown timed out")
		}
		h.sink.Close()
		h.closeErr = errors.Join(h.closeErr, os.RemoveAll(h.dir))
	})
	return h.closeErr
}
