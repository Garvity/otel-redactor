# Local validation evidence

Verified on **2026-09-17** on macOS/arm64, Apple M1 (8 logical CPUs), Go 1.27.1.
Collector dependencies: v0.161.0 / v1.67.0. Docker engine: 29.4.0 through OrbStack.
All telemetry fixtures are synthetic. These results describe this local build;
they are not a third-party audit or a record of hosted GitHub Actions execution.

## Completed checks

| Check | Result |
| --- | --- |
| `make check` | Passed formatting, `go vet`, uncached race tests, binary builds, three example config validations |
| `go mod verify` | All modules verified |
| `go mod tidy -diff` | No changes required |
| `go run ./cmd/redactor-demo` | All seven full-trace checks passed; clean Collector shutdown |
| Real OTLP rejection tests | Oversized batch and malformed JSON return HTTP 400; sink receives no rejected data |
| Concurrent integration test | 12 concurrent requests produce independently verified redacted traces |
| Recursive fuzz smoke | 139,349 executions over ~11 seconds, 2 workers; no failures |
| Allocation benchmarks | Three samples each for 1 / 100 / 1,000 spans and copy-only baselines |
| `make container-check` | Image built; config validated as UID 65532 with read-only root, no network, no capabilities |
| `docker compose config --quiet` | Passed |
| `actionlint v1.7.7` | Both GitHub Actions workflows passed |
| `govulncheck v1.8.0` | 0 reachable vulnerabilities, 0 in imported packages; 1 module-only advisory remains (below) |

Statement coverage from `go test -race -count=1 -coverprofile=coverage.out ./...`:

| Package | Coverage |
| --- | --- |
| `processor/otelredactorprocessor` | 90.9% |
| `internal/collector` | 78.6% |
| `internal/demo` | 80.0% |
| Overall, including thin CLI entry points | 77.7% |

The CLI entry points are exercised by build/validation/demo commands outside the
unit coverage instrumentation, so their reported unit statement coverage is 0%.
No coverage threshold is presented as a correctness guarantee.

## Benchmark snapshot

Command:

```sh
go test ./processor/otelredactorprocessor -run '^$' \
  -bench BenchmarkTraceRedaction -benchmem -count=3
```

Median elapsed time and allocation count per operation:

| Spans/batch | Copy only | Copy + redact | Copy allocations | Copy + redact allocations |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 343 ns | 1,494 ns | 19 | 21 |
| 100 | 21,954 ns | 134,790 ns | 1,207 | 1,407 |
| 1,000 | 262,514 ns | 1,410,071 ns | 12,007 | 14,008 |

The difference between these medians is approximately 1.13–1.15 microseconds per
span for this small synthetic policy. Both variants include pdata copying;
fixture construction and regex compilation are outside the timing loop. HTTP,
OTLP decoding/encoding, queues, export, and backend latency are excluded. Use your
own attribute lengths, rule counts, and nesting patterns before setting budgets.

## Dependency review

The initial scan reported two SSH advisories fixed by updating the indirect
`golang.org/x/crypto` dependency from v0.55.0 to **v0.56.0**:
GO-2026-6354 and GO-2026-6355. The full check suite and container validation were
rerun after that update.

The final scan reports **GO-2026-5932** at module level: the unmaintained
`golang.org/x/crypto/openpgp` package. This project does not import that package;
the scanner reports no affected imported packages or reachable symbols. No fixed
version was available in the scan. The containing module is required indirectly
by Collector dependencies. Recheck with `make vuln` whenever dependencies or
imports change. See the [Go advisory](https://pkg.go.dev/vuln/GO-2026-5932).

## Evidence and remaining verification

`demo-output/report.json` contains the complete before/after trace and the
successful checks. `scripts/render_demo.py` renders it into
`output-images/final-output.png`, including a source digest. The image is a
rendered report of actual verified output, not a terminal screenshot.

The local Git repository has no remote configured. Hosted CI, Linux/macOS release
matrix execution, public artifact publication, branch protections, signing, and
long-duration load tests have not been performed. The container was built and
config-validated locally; functional OTLP integration was exercised using the
native Collector build. Go 1.26 compatibility is configured in CI but was not
separately executed with a local Go 1.26 toolchain.
