# Contributing

Keep changes focused and explain the problem, behavior, and relevant tests.
Use synthetic data only: never add real telemetry, credentials, or personal data
to fixtures, issues, screenshots, logs, or benchmarks.

Run `make check` before submitting a change. Add tests for changed behavior,
especially preservation of unrelated telemetry and failures before export.
Update configuration documentation whenever policy semantics change. Do not
broaden supported signals without corresponding traversal and integration tests.

Commit messages use conventional prefixes such as `feat:`, `fix:`, `test:`, and
`docs:`. Generated binaries and local output should remain untracked; image
artifacts belong in `output-images/`.
