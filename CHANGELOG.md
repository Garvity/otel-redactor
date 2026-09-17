# Changelog

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
