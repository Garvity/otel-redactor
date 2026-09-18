# Changelog

## Unreleased — runtime verification (2026-09-18)

- Added real gRPC/HTTP protobuf, TLS export/retry, live metric, and shutdown-flush
  regression tests against the operational Collector pipeline.
- Added a reproducible native/Docker smoke runner for trace processing, invalid
  requests, body-size limits, output privacy, and graceful SIGTERM shutdown.
- Recorded successful runtime verification; application behavior is unchanged.

## Unreleased — initial alpha implementation (2026-09-17)

- Traces-only `otelredactor` Collector processor with exact key deletion and
  whole-string regex redaction across nested attributes and selected text fields.
- Startup validation, bounded traversal, whole-batch policy rejection, explicit
  OTLP InvalidArgument status, and aggregate metrics without payload labels.
- Minimal Collector executable, local/backend/container configurations, and a
  real OTLP demo with an independent full-trace verifier.
- Unit/race/integration/fuzz tests, baseline allocation benchmarks, CI/release
  candidate workflows, dependency checks, and a hardened non-root container.
- Configuration/operations/security documentation and a reproducible output image.

Logs and metrics signals, universal PII detection, signing/publication, and
third-party production certification are outside this initial implementation.
