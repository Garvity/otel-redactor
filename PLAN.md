# otel-redactor implementation plan

Date: 2026-09-17. Status: in progress.

## Goal and boundaries

Build an independent, reusable OpenTelemetry Collector processor in Go that
removes configured attribute keys and replaces entire string values matching
configured regular expressions. The first release supports traces only. It is
a portfolio/learning project, not an official OpenTelemetry component or a
compliance guarantee. The upstream contrib redaction processor already exists;
this project emphasizes recursive values, explicit failure behavior, and
reproducible verification.

## Architecture and behavior

- `processor/otelredactorprocessor`: public config, Collector factory, immutable
  compiled rules, recursive trace transformation, aggregate counter metrics.
- `internal/collector`: a minimal real Collector distribution with OTLP input,
  memory limiter, this processor, batching, OTLP HTTP/debug output, file/env config.
- `cmd/otel-redactor`: distribution entry point and standard Collector commands.
- `cmd/redactor-demo`: synthetic trace fixture and a real OTLP HTTP round trip
  through the Collector, with machine-checked before/after evidence.
- `examples/`: local and container configs; `docs/`: behavior and operating guide.
- `output-images/`: all generated image artifacts, including final demo output.

Rules apply to resource, instrumentation scope, span, event, and link attributes,
including nested maps and arrays. Exact case-sensitive key deletion precedes
value matching. A regex match replaces the whole string with a literal marker;
IDs, timestamps, numeric/boolean values, and unrelated attributes remain intact.
Span names, event names, and status messages are scanned by default. Binary
values and structural metadata are outside string matching and must be called
out in the documentation. No raw telemetry appears in processor logs or metric
labels. Compiled config is immutable across concurrent requests.

Startup rejects empty policies, invalid regexes, invalid limits, duplicate keys,
and empty replacement markers. Runtime traversal has depth/string-size limits.
An exceeded limit rejects the entire batch with a permanent error and never
calls the next consumer. Cancellation also prevents export. Mutating capability
is declared so the Collector can isolate fan-out consumers correctly.

## Milestones and acceptance gates

1. **Project foundation and design**
   - [x] Scope, architecture, threat boundaries, and milestone plan.
   - [x] Go module, license, ignore rules, contribution/security guidance.
   - [x] Independent Git repository.
   - Gate: review directory boundaries and commit the plan before implementation.
2. **Configuration and transformation engine**
   - [x] Strict configuration validation and immutable precompiled rules.
   - [x] Recursive deletion/redaction at every supported trace location.
   - [x] Unit tests for preservation, matching, nesting, limits, and cancellation.
   - Gate: focused Go tests pass.
3. **Collector component and runnable distribution**
   - [ ] Factory, lifecycle, next-consumer errors, and privacy-safe counters.
   - [ ] Version-pinned Collector distribution and validated example configs.
   - [ ] Factory tests, unsupported signals, telemetry assertions.
   - Gate: build, component tests, Collector config validation pass.
4. **Executable demonstration and integration coverage**
   - [ ] Synthetic fixture; OTLP HTTP receiver -> processor -> OTLP HTTP sink.
   - [ ] Assert secrets absent and trace structure/preserved values unchanged.
   - [ ] Test invalid config, whole-batch rejection, shutdown, concurrent inputs.
   - Gate: real network integration suite and demo pass.
5. **Release engineering and operational hardening**
   - [ ] CI, formatting/vet/race checks, fuzzing, allocation benchmarks.
   - [ ] Container packaging, Make targets, dependency updates, release workflow.
   - [ ] Review configuration limits, error paths, mutable state, and dependencies.
   - Gate: full quality checks, fuzz smoke, benchmark, and security scan; record
     any unavailable checks honestly rather than claiming production readiness.
6. **Documentation and final evidence**
   - [ ] README, configuration reference, architecture/operations documentation.
   - [ ] Record measured validation and benchmark results with environment.
   - [ ] Render actual verified demo output into `output-images/` and inspect it.
   - [ ] Complete plan and commit final artifacts with a clean working tree.
   - Gate: documented quick start reproduced, all final checks pass.

## Verification strategy

Use table-driven tests and real pdata structures; compare complete traces where
possible. Tests must cover resources/scopes/spans/events/links, nested arrays,
UTF-8, literal replacement, malformed policy, deep/large input rejection,
concurrent consumers, downstream errors, and counters without sensitive labels.
Run a bounded fuzz test and reproducible benchmarks with fixture creation outside
timing. Integration tests bind ephemeral loopback ports and use readiness checks
and bounded shutdown. Do not contact external telemetry backends.

## Git and evidence policy

Each milestone gets a separate conventional commit after its gate. Author and
committer dates use 2026-09-17 in Asia/Kolkata, with randomized chronological
times per the user's request. Existing Git identity is retained. No remote is
created or pushed. All image output belongs exclusively in `output-images/`;
binaries, caches, raw local demo output, and coverage files are ignored.

## Deferred scope

Logs/metrics pipelines, automated PII discovery, reversible tokenization,
dynamic config management, Kubernetes deployment automation, hosted UI, and
claims of third-party security audit. Regex policies only protect configured
patterns; encoded values and attributes not covered by a rule can survive.
