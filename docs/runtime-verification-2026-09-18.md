# Runtime feature verification — 2026-09-18

The application was rebuilt and executed on macOS/arm64 with Go 1.27.1 and
Docker 29.4.0. No application defects were found in the checks below. New tests
cover previously unverified transport and deployment behavior; no production
application logic or policy defaults changed.

## Feature results

| Feature | Evidence | Result |
| --- | --- | --- |
| Exact key deletion, all five attribute locations | Full expected-trace comparison and engine tests | Pass |
| Nested maps/arrays and preservation of other values | Engine tests and full OTLP fixture verification | Pass |
| Regex redaction, literal replacement, optional text scrubbing | Engine tests and real exported traces | Pass |
| IDs, parent relationships, timestamps and unrelated fields | Independently constructed complete expected trace | Pass |
| Strict config validation, unsupported pipeline signals | Collector validation and factory tests | Pass |
| Depth/string limits, cancellation, downstream errors | Processor tests, permanent error assertions, downstream call checks | Pass |
| Concurrent requests and immutable policy | 12-request real OTLP test, concurrent engine test, race detector | Pass |
| HTTP JSON ingress | Running Collector, native executable, Docker container | HTTP 200; sanitized output |
| HTTP protobuf ingress | Operational pipeline test | HTTP 200; complete exported trace verified |
| gRPC ingress | Operational pipeline test | Accepted; complete exported trace verified |
| gRPC policy rejection | Oversized retained string | InvalidArgument; no export |
| HTTPS backend export | Local TLS backend, trusted temporary CA; normal gzip/protobuf exporter defaults | Pass |
| Exporter retry and queue | Backend returns one HTTP 503, then recovers | Exactly one retry and successful sanitized delivery |
| Batching and shutdown flush | One-hour batch timeout; stop immediately after acceptance | Buffered trace exported on shutdown |
| Live Prometheus counters | Actual `/metrics` scrape | 18 removed, 24 redacted, 2 rejected after transport test |
| Native executable metrics | Actual `/metrics` scrape | 6 removed, 8 redacted, 1 rejected |
| Native and container malformed JSON | HTTP request | HTTP 400 |
| Native and container unsupported Content-Type | `text/plain` request | HTTP 415 |
| Native and container unsupported logs endpoint | `/v1/logs` request | HTTP 404 |
| Native and container HTTP body limit | 4 MiB + 1 byte request | HTTP 400 |
| Native and container whole-batch rejection | Oversized value in second span | HTTP 400; rejected trace absent from export |
| Native and container output privacy | Detailed debug exporter with synthetic inputs only | Redacted output observed; original secrets absent |
| Container isolation | Docker inspection | UID/GID 65532, read-only root filesystem |
| Graceful process shutdown | SIGTERM to native process and container | Both exit 0; native shutdown completion observed |

The live executable/container test uses the shipped example configurations with
ephemeral ports, a 64-byte string limit for the rejection case, and detailed
debug output **only for synthetic fixtures**. The operational Go test derives
its pipeline from `examples/production.yaml`, enables gRPC, uses ephemeral ports,
a temporary trusted CA, and shorter batch/retry timings. It does not disable TLS
certificate verification.

## Commands executed

```sh
make check
make demo
go test ./internal/demo -run 'TestOperational|TestShutdown' -v -count=1
make container-check
python3 scripts/smoke_runtime.py --docker
```

`make check` was rerun after adding the integration tests and passed, including
uncached race tests, vet, formatting, builds, and all three example configurations.
Processor statement coverage remains 90.9%. The runtime script passed 18 checks
across the native executable and container. Its output is available locally at
`demo-output/runtime-checks.json`; `make demo` refreshed the before/after evidence.

Regression coverage lives in `internal/demo/transports_test.go`. The standalone
runtime checks are reproducible using `scripts/smoke_runtime.py` with only Python's
standard library. All temporary application processes and the uniquely named
test container were stopped and removed after the checks.

## Limits of this verification

These checks cover the implemented traces-only features and the documented
supported runtime paths; they do not prove correctness for every possible input
or deployment. Logs/metrics data processing remains intentionally unsupported.
Sustained load, intentional out-of-memory pressure, remote managed backends,
hosted GitHub Actions, and the full cross-platform release matrix were not
executed in this audit. The prior benchmark and dependency-scan results remain
dated September 17; they were not represented as new measurements here.
