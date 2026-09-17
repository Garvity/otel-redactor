package demo

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestRealOTLPRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(result.Before) || !json.Valid(result.After) || len(result.Checks) != 7 {
		t.Fatal("incomplete demo evidence")
	}
}

func TestRealOTLPRejectedAndMalformedRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := start(ctx, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := h.close(); err != nil {
			t.Error(err)
		}
	}()
	td := Fixture(false)
	td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(1).Attributes().PutStr("oversize", strings.Repeat("x", 65))
	body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(td)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{body, []byte(`{"resourceSpans":`)} {
		status, _, err := h.post(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if status != http.StatusBadRequest {
			t.Fatalf("got HTTP %d, expected 400", status)
		}
	}
	// Queueing is disabled: a completed rejected request must never reach sink.
	select {
	case <-h.received:
		t.Fatal("rejected request exported data")
	default:
	}
}

func TestRealOTLPConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := start(ctx, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := h.close(); err != nil {
			t.Error(err)
		}
	}()
	body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(Fixture(false))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			status, _, err := h.post(ctx, body)
			if err != nil || status != http.StatusOK {
				t.Errorf("HTTP %d: %v", status, err)
			}
		})
	}
	wg.Wait()
	for range 12 {
		select {
		case result := <-h.received:
			if err := Verify(result); err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func TestVerifierDetectsLeaksAndUnrelatedChanges(t *testing.T) {
	for _, secret := range []bool{false, true} {
		td := Fixture(secret)
		if secret {
			td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(1).SetName("wrong")
		}
		body, err := (&ptrace.JSONMarshaler{}).MarshalTraces(td)
		if err != nil {
			t.Fatal(err)
		}
		if err := Verify(body); err == nil {
			t.Fatal("corrupted trace accepted")
		}
	}
	if err := Verify([]byte(`invalid`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
