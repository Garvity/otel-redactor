# Architecture and design decisions

```text
OTLP HTTP / gRPC
       |
 memory_limiter   <- refuse work under memory pressure
       |
 otelredactor     <- delete keys, recursively match strings, enforce limits
       |
     batch        <- buffers only data accepted by the redactor
       |
 OTLP HTTP exporter -> telemetry backend
```

The synthetic demo omits batching and the memory limiter to make per-request
acceptance/rejection directly observable. Operational examples include both.

## Package boundaries

- `processor/otelredactorprocessor/config.go`: public policy and validation.
- `engine.go`: immutable compiled policy, per-call counters, pdata traversal.
- `factory.go`: Collector lifecycle adapter and aggregate metric instruments.
- `internal/collector`: component registration and config providers.
- `internal/demo`: synthetic fixture, local OTLP endpoints, readiness/shutdown,
  and a full-trace verifier independent of the transformation implementation.
- `cmd/otel-redactor`: standard Collector CLI; `cmd/redactor-demo`: bounded demo.

The public processor package has no dependency on the custom distribution or
demo. Reuse its factory in an OCB-generated distribution or a Go Collector host.
Collector APIs are pinned to the same release family: v0.161.0 / v1.67.0.
No dynamic shared libraries, background processor goroutines, global mutable
policy, or process-wide exporter credentials are introduced.

## Why explicit whole-value redaction?

A partially masked string can retain sensitive context around a match. A
literal whole-value marker makes tests and audits easier to reason about. This
costs debugging context; applications requiring selective transformations should
evaluate upstream transform/redaction processors. Exact key deletion is fast
and applies to any attribute type, including binary values.

Regexes compile when the component is created and are safe for concurrent use.
The policy is copied into an engine so later caller changes to Config slices do
not alter a running processor. Config validation also compiles patterns to
provide early errors; no expression is compiled per batch.

## Ownership and failure atomicity

The engine mutates each owned pdata batch. Mutating capability is declared to
the Collector, which decides when fan-out copies are necessary. A rejected batch
can be partially modified in memory but is never forwarded by this processor.
This provides atomicity at the pipeline export boundary, not rollback of the
original in-memory object. Every pipeline must include the policy; an independent
pipeline can still export unredacted data.

Depth is bounded to avoid unbounded recursion in our traversal. String byte
length, rule count, and regex length also have limits. These checks run after
OTLP decoding, so ingress limits and a memory budget remain necessary.

## Observability

The processor emits three monotonically increasing counters through the
Collector's MeterProvider:

| Instrument | Meaning |
| --- | --- |
| `otelredactor.attributes_removed` | Deleted map entries in successfully transformed batches |
| `otelredactor.values_redacted` | String values actually changed in successfully transformed batches |
| `otelredactor.batches_rejected` | Engine failures, including canceled processing |

There are no custom metric attributes/labels; named processor instances aggregate
these counters within a Collector process. Rejected partial work is not included
in removal/replacement counts. Successful transformation counters do not prove
that a downstream exporter delivered the batch. The Collector provides its own
processor/receiver/exporter telemetry for that purpose. Prometheus may normalize
instrument names and append type/unit suffixes.

## Tests and tradeoffs

Unit tests compare complete traces while normalizing unordered attribute lists.
They cover data preservation, every attribute location, nested arrays/maps,
configuration validation, literal replacement, cancellation, limits, immutable
rules, lifecycle, next-consumer errors, and privacy-safe metrics. Integration
tests exercise actual OTLP HTTP ingress/egress, HTTP 400 failures, and concurrent
requests. Fuzzing explores arbitrary strings and nesting shapes.

The demo reserves then releases an ephemeral receiver port because the Collector
owns its listener. A competing process could claim it before startup; that rare
condition produces an explicit startup error rather than connecting to an
unverified listener. The sink binds a separate ephemeral loopback port.

Benchmarks include an equal pdata-copy cost in baseline and redaction cases.
They measure a synthetic policy; they are not a throughput SLA or a general
comparison against the upstream processor.

## References

- [Collector component development](https://opentelemetry.io/docs/collector/extend/custom-component/)
- [Collector source](https://github.com/open-telemetry/opentelemetry-collector)
- [Upstream contrib redaction processor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/processor/redactionprocessor)
