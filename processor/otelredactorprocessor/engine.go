package otelredactorprocessor

import (
	"context"
	"errors"
	"regexp"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var (
	errDepth = errors.New("otelredactor: attribute nesting exceeds max_depth; batch rejected")
	errSize  = errors.New("otelredactor: string exceeds max_string_bytes; batch rejected")
)

type counts struct {
	removed  int64
	redacted int64
}

// engine is immutable after construction. Every call owns its counters and data.
type engine struct {
	keys        map[string]struct{}
	patterns    []*regexp.Regexp
	replacement string
	scrubText   bool
	maxDepth    int
	maxBytes    int
}

func newEngine(cfg *Config) (*engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	e := &engine{
		keys:        make(map[string]struct{}, len(cfg.DeleteKeys)),
		replacement: cfg.Replacement, scrubText: cfg.ScrubTextFields,
		maxDepth: cfg.MaxDepth, maxBytes: cfg.MaxStringBytes,
	}
	for _, key := range cfg.DeleteKeys {
		e.keys[key] = struct{}{}
	}
	for _, pattern := range cfg.Patterns {
		// Validation above compiled exactly these expressions already.
		e.patterns = append(e.patterns, regexp.MustCompile(pattern))
	}
	return e, nil
}

func (e *engine) process(ctx context.Context, td ptrace.Traces) (counts, error) {
	var n counts
	if err := ctx.Err(); err != nil {
		return n, err
	}
	for _, rs := range td.ResourceSpans().All() {
		if err := e.redactMap(ctx, rs.Resource().Attributes(), 0, &n); err != nil {
			return n, err
		}
		for _, ss := range rs.ScopeSpans().All() {
			if err := e.redactMap(ctx, ss.Scope().Attributes(), 0, &n); err != nil {
				return n, err
			}
			for _, span := range ss.Spans().All() {
				if err := e.redactMap(ctx, span.Attributes(), 0, &n); err != nil {
					return n, err
				}
				if e.scrubText {
					name, err := e.redactString(span.Name(), &n)
					if err != nil {
						return n, err
					}
					span.SetName(name)
					message, err := e.redactString(span.Status().Message(), &n)
					if err != nil {
						return n, err
					}
					span.Status().SetMessage(message)
				}
				for _, event := range span.Events().All() {
					if err := e.redactMap(ctx, event.Attributes(), 0, &n); err != nil {
						return n, err
					}
					if e.scrubText {
						name, err := e.redactString(event.Name(), &n)
						if err != nil {
							return n, err
						}
						event.SetName(name)
					}
				}
				for _, link := range span.Links().All() {
					if err := e.redactMap(ctx, link.Attributes(), 0, &n); err != nil {
						return n, err
					}
				}
			}
		}
	}
	return n, ctx.Err()
}

func (e *engine) redactMap(ctx context.Context, attrs pcommon.Map, depth int, n *counts) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > e.maxDepth {
		return errDepth
	}
	// Remove whole subtrees without inspecting their values.
	attrs.RemoveIf(func(key string, _ pcommon.Value) bool {
		_, drop := e.keys[key]
		if drop {
			n.removed++
		}
		return drop
	})
	for _, value := range attrs.All() {
		if err := e.redactValue(ctx, value, depth, n); err != nil {
			return err
		}
	}
	return nil
}

func (e *engine) redactValue(ctx context.Context, value pcommon.Value, depth int, n *counts) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch value.Type() {
	case pcommon.ValueTypeStr:
		redacted, err := e.redactString(value.Str(), n)
		if err != nil {
			return err
		}
		value.SetStr(redacted)
	case pcommon.ValueTypeMap:
		return e.redactMap(ctx, value.Map(), depth+1, n)
	case pcommon.ValueTypeSlice:
		if depth+1 > e.maxDepth {
			return errDepth
		}
		for _, child := range value.Slice().All() {
			if err := e.redactValue(ctx, child, depth+1, n); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *engine) redactString(value string, n *counts) (string, error) {
	if len(value) > e.maxBytes {
		return "", errSize
	}
	for _, pattern := range e.patterns {
		if pattern.MatchString(value) {
			if value != e.replacement {
				n.redacted++
			}
			// Literal whole-value replacement prevents capture expansion and
			// avoids leaking context surrounding a matched substring.
			return e.replacement, nil
		}
	}
	return value, nil
}
