# Operating guide

## Before handling real telemetry

1. Inventory which fields contain sensitive data and remove them at their source
   wherever possible. Write synthetic regression fixtures for your policy.
2. Review the exclusions in [configuration.md](configuration.md). Regex matching
   covers strings only and does not decode base64 or identify every PII format.
3. Validate the full config with `otel-redactor validate --config <path>`.
4. Include `otelredactor` before batching/export queues in **every** traces
   pipeline. Review connectors and secondary pipelines for bypasses.
5. Provision ingress TLS/authentication or a trusted loopback-only deployment.
   The examples intentionally do not expose an unauthenticated public service.
6. Review receiver size limits, the memory limiter, queue bounds, CPU budgets,
   and backend delivery/retry behavior under representative load.

`examples/production.yaml` is a starting template, not a complete deployment
policy. HTTPS export verifies certificates using system roots. Mount a CA bundle
and configure exporter TLS settings if using a private CA. Never put access
tokens directly in committed YAML. The shipped distribution does not include an
authentication extension; add the appropriate factory if one is needed.

## Observe and troubleshoot

- Local example metrics: `http://127.0.0.1:8888/metrics`.
- Rising rejection counts: examine whether configured depth/size limits fit
  legitimate traffic; fix the emitting instrumentation or explicitly adjust
  limits. Processor errors contain no payload values.
- Missing spans with no redactor rejection: inspect receiver, memory limiter,
  exporter, and backend errors; transformation counts are not delivery receipts.
- Empty policy or invalid regex: startup fails. There is no permissive fallback.
- Unsupported logs/metrics pipeline: config/component creation fails rather than
  forwarding those signals through this processor unmodified.
- HTTP 400 for a safety-limit violation: the whole request's trace batch was
  rejected. Repeating the same payload/policy will not help.
- Upstream temporary refusal or backend trouble: retain the Collector's normal
  retry/backpressure handling and review bounded queue consumption.

Default local/debug configurations print basic summaries. Do not enable detailed
debug output on real telemetry until residual field exposure has been reviewed.
Likewise, do not use Collector config-printing commands with confidential config
values in shared terminals or CI logs.

## Shutdown and deployment

The CLI uses the Collector's standard SIGINT/SIGTERM graceful shutdown. Compose
grants a 15-second stop window. Set deployment termination grace to cover your
batch/export timeout and backend conditions; data delivery during a backend outage
is not guaranteed. The example export queue is in memory, so a process crash loses
queued data. The image has no health-check extension or shell-based health probe;
add an appropriate health endpoint for your orchestrator if needed.

The non-root scratch image contains the binary, CA certificates, and one config.
Use a read-only root filesystem, dropped capabilities, and explicit resource
limits as in `compose.yaml`. The Docker build uses an exact Go image digest;
Dependabot proposes base-image and module/action updates for review.

## Release procedure

1. Run `make check`, `make demo`, `make fuzz`, `make bench`, `make vuln`, and
   `make container-check` on the release candidate.
2. Review any module-level vulnerability reports even when reachable-code scans
   pass. Recheck the database for each release.
3. Update the changelog and document any policy behavior changes.
4. Push a reviewed version tag after the repository is hosted. The release
   workflow builds checksummed Linux/macOS amd64/arm64 candidate archives.
5. Inspect artifacts and publish a release through the maintainer's process.
   Publication, signing/attestation, branch protection, and GitHub private
   vulnerability reporting are hosting steps not provisioned by this local build.

CI actions are pinned to commits and use read-only repository permissions. The
release workflow uploads artifacts without a repository write token. This keeps
building a release candidate separate from publishing one.
