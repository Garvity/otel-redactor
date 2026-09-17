# Security boundaries

This project is an unaudited trace processor, not a complete sensitive-data
discovery system. It protects only the explicitly configured keys and patterns.
Binary values, encoded secrets, attribute key names, trace state, instrumentation
identity, schema URLs, and other structural metadata are not automatically
sanitized. Remove sensitive data at instrumentation time wherever possible.

Keep raw telemetry out of public reports. Report reproducible issues using
synthetic inputs. Once the repository is hosted on GitHub, enable private
vulnerability reporting before accepting reports involving confidential details.
Until then, coordinate a private reporting channel with the maintainer; none is
provisioned by this local repository.

Put the processor before batch queues and every exporter. Every output pipeline
must include it; parallel pipelines can otherwise bypass the policy. Invalid
configuration must prevent startup, and runtime safety-limit failures must
prevent the entire affected batch from being exported. Do not enable verbose
debug exporters on real data without reviewing residual exposure.
