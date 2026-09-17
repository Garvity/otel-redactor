# Configuration reference

Collector component ID: `otelredactor` (named instances such as
`otelredactor/customer` are supported). Supported pipeline signal: traces only.
Register `otelredactorprocessor.NewFactory()` to use it in another distribution;
the supplied executable already registers it. A stock Collector cannot load this
package as a runtime plugin. The module path is
`github.com/Garvity/otel-redactor`; this local build does not imply a published module.

| Field | Default | Validation / behavior |
| --- | --- | --- |
| `delete_keys` | `[]` | Up to 1,024 unique, nonblank exact keys; at most 4,096 bytes per key |
| `patterns` | `[]` | Up to 128 valid Go regular expressions; each 1–4,096 bytes |
| `replacement` | `[REDACTED]` | Nonblank literal string, no larger than `max_string_bytes` |
| `scrub_text_fields` | `true` | Scan span names, event names, and span status messages |
| `max_depth` | `32` | Integer 1–128; top-level attribute map is depth 0 |
| `max_string_bytes` | `1048576` | Integer 1–16,777,216; byte length, not Unicode character count |

At least one key or pattern is required. Collector config decoding rejects
unknown fields. Validation errors identify rule indexes without echoing regexes
or keys. Programmatic users should begin with `DefaultConfig()` and then set
their policy; a zero-value `Config` is intentionally invalid.

## Evaluation order

1. Remove matching keys from each attribute map, including nested maps. Matching
   is case-sensitive and exact: `password` does not delete `Password` or
   `user.password`. Dots are literal characters, not path separators.
2. Traverse retained map/array values. Each nested container increases depth by
   one; scalars do not. Empty containers still count toward depth.
3. Check retained string byte length before matching; an exceeded limit rejects
   the whole batch, even if the string would have matched a redaction pattern.
4. On the first regex match, replace the **entire** string with the marker. Regex
   capture references such as `$1` in the marker remain literal. No partial
   string, hashing, or reversible tokenization mode is provided.

Deleted subtrees are not inspected, so a large/deep value under a deleted key
can be discarded safely without hitting the traversal limit. A deleted map
entry counts once regardless of how many values its subtree contains.

Patterns apply to all retained string attribute values without a key allowlist.
Go regexes are unanchored unless you use `^` and `$`; they use RE2-style syntax
without backreferences or lookbehind. An expression such as `.*` matches even
empty strings. Policy matching does not promise universal PII detection.

## Coverage and exclusions

| Location | Key deletion | String matching |
| --- | --- | --- |
| Resource, scope, span, event, link attribute maps | Yes | Yes, recursively |
| Array string values and maps inside arrays | Keys inside maps | Yes |
| Span name, event name, status message | No | If `scrub_text_fields: true` |
| Integer, double, boolean, empty and bytes values | Via their containing key | No |
| Attribute key names | Exact deletion only | No |
| Trace IDs, span IDs, parent IDs, timestamps, flags, status code | No | No |
| Trace state, schema URLs, scope name/version and other structural metadata | No | No |

Numeric identifiers, binary/encoded secrets, and sensitive data in excluded
metadata can survive. Delete their containing attribute keys or stop collecting
them upstream. Disabling text scrubbing also disables byte-limit checks on those
text fields. Attribute limits remain active even with key-deletion-only policies.

## Errors and ownership

Invalid configuration prevents Collector startup. Depth/size violations return
a permanent Collector error with gRPC `InvalidArgument` (HTTP 400 through this
distribution's OTLP receiver). The entire batch is rejected before the next
consumer; partial success is not reported. A caller cancellation returns the
context error. Downstream errors propagate unchanged.

Processing mutates pdata in place. The factory declares `MutatesData: true`, so
Collector fan-out handles ownership isolation. On an error, some input may
already be redacted; there is no rollback. Do not reuse a failed mutable batch
as an original audit copy or concurrently share the same pdata object. Parallel
requests must own separate batches.

Limits are per retained string/container, not a global CPU budget. The examples
also enforce a 4 MiB HTTP body limit and a memory limiter. Review gRPC receive
limits, upstream rate limits, and deployment resource budgets for your load.
