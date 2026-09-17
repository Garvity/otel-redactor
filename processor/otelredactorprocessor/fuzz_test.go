package otelredactorprocessor

import (
	"context"
	"errors"
	"testing"
)

func FuzzRecursiveRedaction(f *testing.F) {
	f.Add("alice@example.test", uint8(0), false)
	f.Add("hello 世界", uint8(4), true)
	f.Add("", uint8(20), false)
	f.Add("\x00\xffalice@example.test", uint8(1), true)
	cfg := policy()
	cfg.MaxDepth = 8
	cfg.MaxStringBytes = 4096
	e := mustEngine(f, cfg)
	f.Fuzz(func(t *testing.T, value string, nesting uint8, useArrays bool) {
		if len(value) > 8192 {
			t.Skip()
		}
		td, span := traceWithSpan()
		attrs := span.Attributes()
		attrs.PutStr("secret", value)
		for range int(nesting % 24) {
			if useArrays {
				attrs = attrs.PutEmptySlice("list").AppendEmpty().SetEmptyMap()
			} else {
				attrs = attrs.PutEmptyMap("nested")
			}
		}
		attrs.PutStr("value", value)
		_, err := e.process(context.Background(), td)
		if err != nil {
			if !errors.Is(err, errDepth) && !errors.Is(err, errSize) {
				t.Fatal(err)
			}
			return
		}
		if _, ok := span.Attributes().Get("secret"); ok {
			t.Fatal("configured key survived")
		}
		got, ok := attrs.Get("value")
		if !ok {
			t.Fatal("unrelated key removed")
		}
		want := value
		if e.patterns[0].MatchString(value) {
			want = cfg.Replacement
		}
		if got.Str() != want {
			t.Fatal("redaction or preservation invariant broken")
		}
		if n, err := e.process(context.Background(), td); err != nil || n != (counts{}) {
			t.Fatalf("second pass changed sanitized input: %+v %v", n, err)
		}
	})
}
