package otelredactorprocessor

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func BenchmarkTraceRedaction(b *testing.B) {
	for _, spans := range []int{1, 100, 1000} {
		input := ptrace.NewTraces()
		ss := input.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
		for range spans {
			span := ss.Spans().AppendEmpty()
			span.SetName("POST /checkout")
			span.Attributes().PutStr("secret", "synthetic-token")
			span.Attributes().PutStr("contact", "alice@example.test")
			span.Attributes().PutInt("http.response.status_code", 200)
			span.Attributes().PutEmptyMap("customer").PutEmptySlice("contacts").AppendEmpty().SetStr("bob@example.test")
		}
		for _, process := range []bool{false, true} {
			name := "CopyOnly"
			if process {
				name = "CopyAndRedact"
			}
			b.Run(fmt.Sprintf("%s/%d_spans", name, spans), func(b *testing.B) {
				e := mustEngine(b, policy())
				ctx := context.Background()
				b.ReportAllocs()
				// Fixture construction/regex compilation are outside timing. Both
				// variants include identical pdata copying needed for mutable input.
				for b.Loop() {
					td := ptrace.NewTraces()
					input.CopyTo(td)
					if process {
						if _, err := e.process(ctx, td); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.ReportMetric(float64(spans), "spans/op")
			})
		}
	}
}
