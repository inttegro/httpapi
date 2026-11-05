package endpoint

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCachedAvailabilityEvaluatorCachesByOperation(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	calls := 0
	cache := NewCachedAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		calls++
		return EndpointAvailabilityDecision{Available: false, Reason: "incident"}, nil
	}), 5*time.Second)
	cache.Now = func() time.Time { return now }

	evaluation := EndpointAvailabilityEvaluation{OperationID: "createWidget"}
	for range 2 {
		decision, err := cache.EvaluateEndpointAvailability(context.Background(), evaluation)
		if err != nil || decision.Available {
			t.Fatalf("decision = %#v error = %v", decision, err)
		}
	}
	if calls != 1 {
		t.Fatalf("evaluator calls = %d, want 1", calls)
	}

	now = now.Add(6 * time.Second)
	if _, err := cache.EvaluateEndpointAvailability(context.Background(), evaluation); err != nil {
		t.Fatalf("refresh decision: %v", err)
	}
	if calls != 2 {
		t.Fatalf("evaluator calls after ttl = %d, want 2", calls)
	}
}

func TestCachedAvailabilityEvaluatorRetainsStaleDecisionOnError(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	fail := false
	cache := NewCachedAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		if fail {
			return EndpointAvailabilityDecision{}, errors.New("storage unavailable")
		}
		return EndpointAvailabilityDecision{Available: false, Reason: "incident"}, nil
	}), time.Second)
	cache.Now = func() time.Time { return now }

	evaluation := EndpointAvailabilityEvaluation{OperationID: "createWidget"}
	if _, err := cache.EvaluateEndpointAvailability(context.Background(), evaluation); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	fail = true
	now = now.Add(2 * time.Second)

	decision, err := cache.EvaluateEndpointAvailability(context.Background(), evaluation)
	if err != nil {
		t.Fatalf("stale decision returned error: %v", err)
	}
	if decision.Available || decision.Reason != "incident" {
		t.Fatalf("stale decision = %#v", decision)
	}
}
