package otelredactorprocessor

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/otel/metric"
)

var componentType = component.MustNewType("otelredactor")

// NewFactory registers the traces-only processor. Logs and metrics are rejected
// by the Collector rather than silently passing through unsanitized.
func NewFactory() processor.Factory {
	return processor.NewFactory(componentType,
		func() component.Config { return DefaultConfig() },
		processor.WithTraces(createTraces, component.StabilityLevelAlpha))
}

func createTraces(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	config, ok := cfg.(*Config)
	if !ok {
		return nil, errors.New("otelredactor: unexpected configuration type")
	}
	e, err := newEngine(config)
	if err != nil {
		return nil, err
	}
	meter := set.MeterProvider.Meter("github.com/Garvity/otel-redactor/processor/otelredactorprocessor")
	removed, err := meter.Int64Counter("otelredactor.attributes_removed", metric.WithDescription("Attribute entries deleted in successfully transformed batches"), metric.WithUnit("{attribute}"))
	if err != nil {
		return nil, err
	}
	redacted, err := meter.Int64Counter("otelredactor.values_redacted", metric.WithDescription("String values changed in successfully transformed batches"), metric.WithUnit("{value}"))
	if err != nil {
		return nil, err
	}
	rejected, err := meter.Int64Counter("otelredactor.batches_rejected", metric.WithDescription("Batches rejected by this processor, including cancellation"), metric.WithUnit("{batch}"))
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTraces(ctx, set, cfg, next,
		func(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
			n, err := e.process(ctx, td)
			if err != nil {
				rejected.Add(ctx, 1)
				if errors.Is(err, errDepth) || errors.Is(err, errSize) {
					err = consumererror.NewPermanent(err)
				}
				return ptrace.Traces{}, err
			}
			removed.Add(ctx, n.removed)
			redacted.Add(ctx, n.redacted)
			return td, nil
		}, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}
