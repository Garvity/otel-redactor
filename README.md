# otel-redactor

A Go OpenTelemetry Collector processor for explicit, recursive trace redaction.

Implementation is organized into independently verified milestones in
[PLAN.md](PLAN.md). The initial release will include a reusable processor, a
minimal Collector distribution, and a synthetic end-to-end OTLP demonstration.

This is an independent learning project. OpenTelemetry already provides a
[contrib redaction processor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/redactionprocessor).

License: Apache-2.0.
