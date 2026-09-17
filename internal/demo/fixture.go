// Package demo runs a synthetic OTLP round trip and verifies the exported data.
package demo

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Fixture constructs deterministic synthetic input, or its expected redacted
// equivalent. The expected output is constructed independently of the processor.
func Fixture(redacted bool) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout-demo")
	putSecret(rs.Resource().Attributes(), "authorization", redacted)
	putEmail(rs.Resource().Attributes(), "owner", redacted)
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("redactor-demo")
	ss.Scope().SetVersion("1.0.0")
	putSecret(ss.Scope().Attributes(), "api.key", redacted)
	putEmail(ss.Scope().Attributes(), "contact", redacted)
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetName("POST /checkout")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(1789642800000000000)
	span.SetEndTimestamp(1789642800120000000)
	span.Attributes().PutInt("http.response.status_code", 200)
	span.Attributes().PutBool("payment.approved", true)
	putSecret(span.Attributes(), "password", redacted)
	putEmail(span.Attributes(), "user.email", redacted)
	nested := span.Attributes().PutEmptyMap("customer")
	putSecret(nested, "password", redacted)
	list := nested.PutEmptySlice("contacts")
	list.AppendEmpty().SetStr(emailValue(redacted))
	list.AppendEmpty().SetInt(42)
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.Status().SetMessage(emailValue(redacted))
	event := span.Events().AppendEmpty()
	event.SetName(emailValue(redacted))
	event.SetTimestamp(1789642800050000000)
	putSecret(event.Attributes(), "authorization", redacted)
	putEmail(event.Attributes(), "message", redacted)
	link := span.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID{2})
	link.SetSpanID(pcommon.SpanID{3})
	putSecret(link.Attributes(), "api.key", redacted)
	putEmail(link.Attributes(), "owner", redacted)
	child := ss.Spans().AppendEmpty()
	child.SetTraceID(span.TraceID())
	child.SetParentSpanID(span.SpanID())
	child.SetSpanID(pcommon.SpanID{9})
	child.SetName("inventory.reserve")
	child.SetKind(ptrace.SpanKindInternal)
	child.SetStartTimestamp(1789642800010000000)
	child.SetEndTimestamp(1789642800100000000)
	child.Attributes().PutStr("sku", "DEMO-001")
	child.Attributes().PutInt("quantity", 2)
	return td
}

func putSecret(attrs pcommon.Map, key string, redacted bool) {
	if !redacted {
		attrs.PutStr(key, "synthetic-secret-do-not-use")
	}
}

func putEmail(attrs pcommon.Map, key string, redacted bool) { attrs.PutStr(key, emailValue(redacted)) }

func emailValue(redacted bool) string {
	if redacted {
		return "[REDACTED]"
	}
	return "alice@example.test"
}
