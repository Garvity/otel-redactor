# otel-redactor

**Policy-driven trace redaction, built into the telemetry pipeline.**

A Go processor and custom OpenTelemetry Collector distribution that remove
configured secrets before traces reach an observability backend. It handles
nested attributes, preserves trace structure, and rejects entire batches when
configured safety limits are exceeded.

**Go · OpenTelemetry · OTLP · gRPC · Prometheus · Docker · GitHub Actions**

![Verified redaction through a real OTLP pipeline](output-images/final-output.png)

## The problem

Distributed traces can carry email addresses, authorization headers, API keys,
and other sensitive values across service boundaries. Those values can appear
in resource metadata, nested attributes, span events, and status messages.

`otel-redactor` applies one explicit policy across these locations before export.
It retains the identifiers, timing, relationships, and unrelated attributes
needed to investigate application behavior.

## Engineering highlights

| Capability | Implementation |
| --- | --- |
| Recursive policy evaluation | Exact key deletion and whole-string regex replacement across resource, scope, span, event, and link attributes, including nested maps and arrays |
| Predictable failure behavior | Invalid policies prevent startup; depth and string-size violations reject the entire batch before downstream processing |
| Concurrent processing | Immutable compiled rules, request-local counters, and declared pdata mutation semantics for Collector fan-out |
| Observable processing | Aggregate removal, replacement, and rejection counters without payload-derived metric labels |
| Transport integration | OTLP HTTP JSON/protobuf and gRPC input; HTTPS export with batching, bounded queues, and retries |
| Container isolation | Non-root scratch image, pinned build-image digest, read-only filesystem, dropped capabilities, and explicit resource limits |
| Release tooling | Pinned CI actions, dependency checks, and checksummed release-candidate archives for Linux/macOS on amd64/arm64 |

## Verified behavior

The test suite checks both **what is removed** and **what must remain unchanged**.
An end-to-end demo compares the complete exported trace against an independently
constructed expected result.

| Verification | Recorded result |
| --- | --- |
| Processor statement coverage | **90.9%** |
| Recursive fuzz testing | **139,349 inputs** in a bounded run without failure |
| Native executable and Docker smoke checks | **18 checks passed** |
| Concurrency | Race tests and a 12-request concurrent OTLP integration test passed |
| Transport and recovery | HTTP JSON/protobuf, gRPC, TLS export, retry after HTTP 503, and shutdown flushing passed |
| Synthetic demonstration | **6 secret attributes removed, 8 strings redacted, 2 spans preserved** |

Measurements were recorded on September 17–18, 2026. See the
[validation results](docs/validation.md) and
[runtime verification report](docs/runtime-verification-2026-09-18.md) for commands,
environment details, and test boundaries. Hosted CI and the cross-platform release
matrix are configured but have not yet been executed.

## Try it locally

Requires Go 1.26 or later and network access for the initial dependency download.
Race tests also require a C compiler. Docker is optional.

```sh
make demo
```

This starts a real Collector and an OTLP HTTP sink on ephemeral loopback ports,
sends synthetic traces, verifies the result, and shuts down both services.

```text
PASS 6 configured secret attributes removed, including nested password
PASS 8 matching strings redacted across attributes and text fields
PASS 2 spans preserved with original IDs, parent relationship, and timestamps
PASS Complete exported trace matches the independently constructed expectation
PASS Collector shut down cleanly
```

Inspect `demo-output/before.json`, `after.json`, and `report.json` for the complete
evidence. The image above is rendered from that verified output; generated images
live in `output-images/`.

## Architecture

```text
Application traces
       |
       v
OTLP receiver -> Memory limiter -> otelredactor -> Batch -> OTLP exporter
                                       |
                                       +-- Aggregate processing metrics
```

Redaction runs before batching and export queues. The processor compiles regexes
at initialization and keeps its policy immutable during request processing.
Each request owns its telemetry and counters. A policy-limit violation returns
an explicit permanent error: HTTP 400 or gRPC `InvalidArgument` through the
OTLP receiver.

The public processor package can be registered in another Collector distribution
using `otelredactorprocessor.NewFactory()`. The included executable provides a
ready-to-run distribution with local, backend-export, and container configs.
See [architecture and tradeoffs](docs/architecture.md).

## Configure a policy

```yaml
processors:
  otelredactor:
    delete_keys: [authorization, api.key, password]
    patterns:
      - '(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}'
      - '(?i)bearer\s+[a-z0-9._~+/=\-]+'
    replacement: '[REDACTED]'
    scrub_text_fields: true
    max_depth: 32
    max_string_bytes: 1048576
```

Key deletion is exact and case-sensitive. A regex match replaces the entire
string with a literal marker. Span names, event names, and status messages are
scanned by default. The example patterns illustrate a policy; protection depends
on the keys and expressions you configure.

Read the [configuration reference](docs/configuration.md) for evaluation order,
limits, supported locations, and error semantics.

## Run the Collector

```sh
make build
./bin/otel-redactor validate --config examples/collector.yaml
./bin/otel-redactor --config examples/collector.yaml
```

| Endpoint | Address |
| --- | --- |
| OTLP HTTP | `127.0.0.1:4318` |
| OTLP gRPC | `127.0.0.1:4317` |
| Internal Prometheus metrics | `127.0.0.1:8888/metrics` |

The local configuration exports basic debug summaries. After running `make demo`,
send its synthetic fixture to the running Collector:

```sh
curl --fail-with-body -H 'Content-Type: application/json' \
  --data-binary @demo-output/before.json http://127.0.0.1:4318/v1/traces
```

For HTTPS backend export, review [examples/production.yaml](examples/production.yaml)
and set `OTEL_REDACTOR_BACKEND` to the backend's base URL. Ingress binds to loopback;
remote ingestion requires explicit TLS and authentication configuration.

### Docker

```sh
make container-check
docker compose up --build
```

Compose publishes HTTP input on host loopback and applies memory, CPU, and PID
limits. The image runs as UID/GID 65532 and includes CA certificates for HTTPS
export. Stop the service with `docker compose down`.

## Performance snapshot

Median batch processing times from three benchmark samples on an Apple M1,
Go 1.27.1, macOS/arm64:

| Spans per batch | Copy baseline | Copy + redaction |
| ---: | ---: | ---: |
| 1 | 0.343 µs | 1.494 µs |
| 100 | 21.954 µs | 134.790 µs |
| 1,000 | 262.514 µs | 1,410.071 µs |

For this synthetic policy, the difference is approximately **1.13–1.15 µs per
span**. Both cases include the same pdata-copy step. Transport, encoding,
decoding, queues, and backend latency are excluded. Reproduce with `make bench`;
see [benchmark methodology](docs/validation.md#benchmark-snapshot) before applying
these numbers to a different workload.

## Development

| Command | Checks |
| --- | --- |
| `make check` | Formatting, vet, race tests, builds, and example configuration validation |
| `make demo` | Actual OTLP round trip with full before/after verification |
| `make fuzz` | Recursive redaction invariants over generated inputs |
| `make bench` | Runtime and allocation benchmarks with a copy baseline |
| `make vuln` | Dependency vulnerability scan |
| `make container-check` | Image build and isolated non-root configuration validation |

After building the binaries and image and running the demo, exercise both runtimes:

```sh
python3 scripts/smoke_runtime.py --docker
```

The script uses Python's standard library, tests valid and invalid requests,
checks output and metrics, and verifies graceful shutdown. It cleans up its
temporary processes and container and writes `demo-output/runtime-checks.json`.

## Scope and documentation

Current status: **alpha, traces-only**. Binary values, encoded secrets, attribute
key names, trace state, schema URLs, instrumentation identity, and unconfigured
patterns are outside automatic string redaction. Logs and metrics processing
are unsupported. Sustained-load and remote production deployments have not
been validated.

- [Architecture and design decisions](docs/architecture.md)
- [Configuration reference](docs/configuration.md)
- [Operating guide](docs/operations.md)
- [Security boundaries](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

Licensed under [Apache-2.0](LICENSE).
