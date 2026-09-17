package otelredactorprocessor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestFactoryLifecycleAndUnsupportedSignals(t *testing.T) {
	f := NewFactory()
	set := processortest.NewNopSettings(f.Type())
	ctx := context.Background()
	if _, err := f.CreateTraces(ctx, set, f.CreateDefaultConfig(), consumertest.NewNop()); err == nil {
		t.Fatal("empty policy accepted")
	}
	if _, err := createTraces(ctx, set, struct{}{}, consumertest.NewNop()); err == nil {
		t.Fatal("wrong config type accepted")
	}
	if _, err := f.CreateMetrics(ctx, set, policy(), consumertest.NewNop()); err == nil {
		t.Fatal("metrics supported unexpectedly")
	}
	if _, err := f.CreateLogs(ctx, set, policy(), consumertest.NewNop()); err == nil {
		t.Fatal("logs supported unexpectedly")
	}
	p, err := f.CreateTraces(ctx, set, policy(), consumertest.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if !p.Capabilities().MutatesData {
		t.Fatal("mutation must be declared")
	}
	if err := p.Start(ctx, componenttest.NewNopHost()); err != nil {
		t.Fatal(err)
	}
	if err := p.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRejectedBatchNeverReachesNextConsumer(t *testing.T) {
	for _, failure := range []string{"size", "depth", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			called := false
			next, err := consumer.NewTraces(func(context.Context, ptrace.Traces) error { called = true; return nil })
			if err != nil {
				t.Fatal(err)
			}
			cfg := policy()
			cfg.MaxStringBytes = 20
			cfg.MaxDepth = 1
			p, err := NewFactory().CreateTraces(context.Background(), processortest.NewNopSettings(componentType), cfg, next)
			if err != nil {
				t.Fatal(err)
			}
			td, first := traceWithSpan()
			first.Attributes().PutStr("secret", "removed-before-error")
			second := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().AppendEmpty()
			ctx := context.Background()
			switch failure {
			case "size":
				second.Attributes().PutStr("email", strings.Repeat("x", 21))
			case "depth":
				second.Attributes().PutEmptyMap("a").PutEmptyMap("b")
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err = p.ConsumeTraces(ctx, td)
			if err == nil || called {
				t.Fatalf("failed batch reached downstream: called=%v err=%v", called, err)
			}
			if failure != "cancel" && !consumererror.IsPermanent(err) {
				t.Fatal("limit failure must be permanent")
			}
			if failure == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestDownstreamErrorPropagates(t *testing.T) {
	want := errors.New("downstream unavailable")
	next, err := consumer.NewTraces(func(_ context.Context, td ptrace.Traces) error {
		if _, ok := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes().Get("secret"); ok {
			t.Error("secret reached downstream")
		}
		return want
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewFactory().CreateTraces(context.Background(), processortest.NewNopSettings(componentType), policy(), next)
	if err != nil {
		t.Fatal(err)
	}
	td, span := traceWithSpan()
	span.Attributes().PutStr("secret", "test-token")
	if err := p.ConsumeTraces(context.Background(), td); !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestCountersContainNoTelemetryLabels(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer provider.Shutdown(ctx)
	set := processortest.NewNopSettings(componentType)
	set.MeterProvider = provider
	cfg := policy()
	cfg.MaxStringBytes = 32
	p, err := NewFactory().CreateTraces(ctx, set, cfg, consumertest.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	td, span := traceWithSpan()
	span.Attributes().PutStr("secret", "synthetic-token")
	span.Attributes().PutStr("contact", "alice@example.test")
	if err := p.ConsumeTraces(ctx, td); err != nil {
		t.Fatal(err)
	}
	td, span = traceWithSpan()
	span.Attributes().PutStr("oversize", strings.Repeat("x", 33))
	if err := p.ConsumeTraces(ctx, td); err == nil {
		t.Fatal("expected rejection")
	}
	var result metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &result); err != nil {
		t.Fatal(err)
	}
	found := map[string]int64{}
	for _, scope := range result.ScopeMetrics {
		for _, m := range scope.Metrics {
			if !strings.HasPrefix(m.Name, "otelredactor.") {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("unexpected metric type %T", m.Data)
			}
			for _, point := range sum.DataPoints {
				if point.Attributes.Len() != 0 {
					t.Fatal("redaction metrics must not expose user-derived labels")
				}
				found[m.Name] += point.Value
			}
		}
	}
	for _, name := range []string{"otelredactor.attributes_removed", "otelredactor.values_redacted", "otelredactor.batches_rejected"} {
		if found[name] != 1 {
			t.Errorf("%s=%d want 1", name, found[name])
		}
	}
}
