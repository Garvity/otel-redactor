package otelredactorprocessor

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func traceWithSpan() (ptrace.Traces, ptrace.Span) {
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("checkout")
	span.SetTraceID(pcommon.TraceID{1, 2, 3})
	span.SetSpanID(pcommon.SpanID{4, 5, 6})
	span.SetStartTimestamp(100)
	span.SetEndTimestamp(200)
	return td, span
}

func mustEngine(t testing.TB, cfg *Config) *engine {
	t.Helper()
	e, err := newEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEveryAttributeLocationAndPreservation(t *testing.T) {
	td, span := traceWithSpan()
	rs := td.ResourceSpans().At(0)
	ss := rs.ScopeSpans().At(0)
	rs.SetSchemaUrl("https://example.test/schema")
	ss.Scope().SetName("example-instrumentation")
	span.SetParentSpanID(pcommon.SpanID{9})
	span.TraceState().FromRaw("vendor=value")
	span.SetKind(ptrace.SpanKindServer)
	span.SetDroppedAttributesCount(7)
	event := span.Events().AppendEmpty()
	event.SetName("payment")
	event.SetTimestamp(150)
	link := span.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID{8})
	link.SetSpanID(pcommon.SpanID{7})
	locations := []pcommon.Map{rs.Resource().Attributes(), ss.Scope().Attributes(), span.Attributes(), event.Attributes(), link.Attributes()}
	for _, attrs := range locations {
		attrs.PutStr("secret", "synthetic-token")
		attrs.PutStr("contact", "alice@example.test")
		attrs.PutStr("safe", "hello 世界")
		attrs.PutInt("count", 42)
		attrs.PutBool("ok", true)
		attrs.PutDouble("ratio", 1.5)
		attrs.PutEmpty("empty")
		attrs.PutEmptyBytes("binary").FromRaw([]byte{1, 2, 3})
		attrs.PutStr("Secret", "case-sensitive-key-survives")
		nested := attrs.PutEmptyMap("nested")
		nested.PutStr("secret", "nested-token")
		list := nested.PutEmptySlice("items")
		list.AppendEmpty().SetStr("bob@example.test")
		list.AppendEmpty().SetInt(10)
		list.AppendEmpty().SetEmptyMap().PutStr("secret", "deep-token")
	}
	want := ptrace.NewTraces()
	td.CopyTo(want)
	wrs := want.ResourceSpans().At(0)
	wss := wrs.ScopeSpans().At(0)
	wsp := wss.Spans().At(0)
	for _, attrs := range []pcommon.Map{wrs.Resource().Attributes(), wss.Scope().Attributes(), wsp.Attributes(), wsp.Events().At(0).Attributes(), wsp.Links().At(0).Attributes()} {
		attrs.Remove("secret")
		attrs.PutStr("contact", "[REDACTED]")
		nested, _ := attrs.Get("nested")
		nested.Map().Remove("secret")
		items, _ := nested.Map().Get("items")
		items.Slice().At(0).SetStr("[REDACTED]")
		items.Slice().At(2).Map().Remove("secret")
	}
	n, err := mustEngine(t, policy()).process(context.Background(), td)
	if err != nil {
		t.Fatal(err)
	}
	if n != (counts{removed: 15, redacted: 10}) {
		t.Fatalf("unexpected counts: %+v", n)
	}
	marshaler := ptrace.JSONMarshaler{}
	gotJSON, _ := marshaler.MarshalTraces(td)
	wantJSON, _ := marshaler.MarshalTraces(want)
	if !reflect.DeepEqual(canonicalTraceJSON(t, gotJSON), canonicalTraceJSON(t, wantJSON)) {
		t.Fatalf("trace differs beyond intended changes:\ngot %s\nwant %s", gotJSON, wantJSON)
	}
}

// OTLP represents unordered attribute maps as lists. Compare their contents
// independently of Remove versus RemoveIf's different compaction order.
func canonicalTraceJSON(t *testing.T, data []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
			if len(x) > 0 {
				if first, ok := x[0].(map[string]any); ok && first["key"] != nil {
					sort.Slice(x, func(i, j int) bool {
						return x[i].(map[string]any)["key"].(string) < x[j].(map[string]any)["key"].(string)
					})
				}
			}
		}
	}
	walk(value)
	return value
}

func TestTextFieldsAndLiteralReplacement(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			td, span := traceWithSpan()
			span.SetName("alice@example.test")
			span.Status().SetMessage("failed for bob@example.test")
			event := span.Events().AppendEmpty()
			event.SetName("carol@example.test")
			cfg := policy()
			cfg.ScrubTextFields = enabled
			cfg.Replacement = "$1-literal"
			n, err := mustEngine(t, cfg).process(context.Background(), td)
			if err != nil {
				t.Fatal(err)
			}
			if enabled {
				if span.Name() != cfg.Replacement || span.Status().Message() != cfg.Replacement || event.Name() != cfg.Replacement || n.redacted != 3 {
					t.Fatal("text not redacted literally")
				}
			} else if span.Name() != "alice@example.test" || n.redacted != 0 {
				t.Fatal("disabled text changed")
			}
		})
	}
}

func TestLimitsAndDeletionPrecedence(t *testing.T) {
	tests := []struct {
		name string
		fill func(pcommon.Map)
		want error
	}{
		{"long string", func(m pcommon.Map) { m.PutStr("safe", strings.Repeat("x", 21)) }, errSize},
		{"at byte limit", func(m pcommon.Map) { m.PutStr("safe", strings.Repeat("x", 20)) }, nil},
		{"deep map", func(m pcommon.Map) { m.PutEmptyMap("a").PutEmptyMap("b").PutStr("x", "x") }, errDepth},
		{"deep array", func(m pcommon.Map) { m.PutEmptySlice("a").AppendEmpty().SetEmptySlice() }, errDepth},
		{"at depth limit", func(m pcommon.Map) { m.PutEmptyMap("a").PutStr("safe", "x") }, nil},
		{"deleted large value", func(m pcommon.Map) { m.PutStr("secret", strings.Repeat("x", 21)) }, nil},
		{"deleted deep tree", func(m pcommon.Map) { m.PutEmptyMap("secret").PutEmptyMap("a").PutEmptyMap("b") }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := policy()
			cfg.MaxDepth = 1
			cfg.MaxStringBytes = 20
			td, span := traceWithSpan()
			tt.fill(span.Attributes())
			_, err := mustEngine(t, cfg).process(context.Background(), td)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v want %v", err, tt.want)
			}
		})
	}
}

func TestCancellationAndEmptyData(t *testing.T) {
	e := mustEngine(t, policy())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	td, _ := traceWithSpan()
	if _, err := e.process(ctx, td); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.process(context.Background(), ptrace.NewTraces()); err != nil {
		t.Fatal(err)
	}
}

func TestImmutablePolicyConcurrentCallsAndRepeat(t *testing.T) {
	cfg := policy()
	e := mustEngine(t, cfg)
	cfg.DeleteKeys[0] = "safe"
	cfg.Patterns[0] = ".*"
	cfg.Replacement = "changed"
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			td, span := traceWithSpan()
			span.Attributes().PutStr("secret", "token")
			span.Attributes().PutStr("safe", "alice@example.test")
			n, err := e.process(context.Background(), td)
			if err != nil || !reflect.DeepEqual(n, counts{1, 1}) {
				t.Errorf("unexpected first pass: %+v %v", n, err)
			}
			n, err = e.process(context.Background(), td)
			if err != nil || n != (counts{}) {
				t.Errorf("unexpected second pass: %+v %v", n, err)
			}
			v, _ := span.Attributes().Get("safe")
			if v.Str() != "[REDACTED]" {
				t.Error("config mutation affected engine")
			}
		})
	}
	wg.Wait()
}
