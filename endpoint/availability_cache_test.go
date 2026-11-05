package endpoint

import (
	"context"
	"errors"
	"strconv"
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

func TestCachedAvailabilityEvaluatorSeparatesScopedSubjects(t *testing.T) {
	calls := 0
	cache := NewCachedAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		_ context.Context,
		evaluation EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		calls++
		return EndpointAvailabilityDecision{
			Available: evaluation.Subject.AppID != "app_blocked",
		}, nil
	}), time.Minute)

	for _, appID := range []string{"app_allowed", "app_blocked", "app_allowed"} {
		decision, err := cache.EvaluateEndpointAvailability(context.Background(), EndpointAvailabilityEvaluation{
			OperationID: "createWidget",
			Phase:       EndpointAvailabilityPhaseScoped,
			Subject:     EndpointAvailabilitySubject{AppID: appID},
		})
		if err != nil {
			t.Fatalf("evaluate %s: %v", appID, err)
		}
		if decision.Available != (appID == "app_allowed") {
			t.Fatalf("decision for %s = %#v", appID, decision)
		}
	}
	if calls != 2 {
		t.Fatalf("evaluator calls = %d, want 2 distinct subject lookups", calls)
	}
}

func TestCachedAvailabilityEvaluatorBoundsScopedSubjectEntries(t *testing.T) {
	cache := NewCachedAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		return EndpointAvailabilityDecision{Available: true}, nil
	}), time.Minute)

	for index := range maxAvailabilityCacheEntries + 1 {
		_, err := cache.EvaluateEndpointAvailability(context.Background(), EndpointAvailabilityEvaluation{
			OperationID: "createWidget",
			Phase:       EndpointAvailabilityPhaseScoped,
			Subject: EndpointAvailabilitySubject{
				AppID: "app_" + strconv.Itoa(index),
			},
		})
		if err != nil {
			t.Fatalf("evaluate scoped subject %d: %v", index, err)
		}
	}
	if len(cache.entries) != maxAvailabilityCacheEntries {
		t.Fatalf("cache entries = %d, want %d", len(cache.entries), maxAvailabilityCacheEntries)
	}
}
