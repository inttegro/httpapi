package endpoint

import (
	"context"
	"strings"
	"sync"
	"time"
)

const maxAvailabilityCacheEntries = 4096

// CachedAvailabilityEvaluator bounds control-plane reads while retaining the
// last known decision through transient storage failures. A cold-cache failure
// is returned to the runtime, which deliberately fails open.
type CachedAvailabilityEvaluator struct {
	Evaluator AvailabilityEvaluator
	TTL       time.Duration
	Now       func() time.Time

	mu      sync.RWMutex
	entries map[string]cachedAvailabilityDecision
}

type cachedAvailabilityDecision struct {
	decision EndpointAvailabilityDecision
	fetched  time.Time
}

// NewCachedAvailabilityEvaluator wraps evaluator with a bounded cache keyed by
// operation, evaluation phase, and trusted request subject.
func NewCachedAvailabilityEvaluator(
	evaluator AvailabilityEvaluator,
	ttl time.Duration,
) *CachedAvailabilityEvaluator {
	return &CachedAvailabilityEvaluator{
		Evaluator: evaluator,
		TTL:       ttl,
		Now:       time.Now,
		entries:   make(map[string]cachedAvailabilityDecision),
	}
}

// EvaluateEndpointAvailability implements AvailabilityEvaluator.
func (e *CachedAvailabilityEvaluator) EvaluateEndpointAvailability(
	ctx context.Context,
	evaluation EndpointAvailabilityEvaluation,
) (EndpointAvailabilityDecision, error) {
	if e == nil || e.Evaluator == nil {
		return EndpointAvailabilityDecision{Available: true, Provider: "default"}, nil
	}

	now := time.Now()
	if e.Now != nil {
		now = e.Now()
	}
	key := availabilityCacheKey(evaluation)

	e.mu.RLock()
	cached, found := e.entries[key]
	e.mu.RUnlock()
	if found && now.Sub(cached.fetched) < e.TTL {
		return cached.decision, nil
	}

	decision, err := e.Evaluator.EvaluateEndpointAvailability(ctx, evaluation)
	if err != nil {
		if found {
			return cached.decision, nil
		}
		return EndpointAvailabilityDecision{}, err
	}

	e.mu.Lock()
	if e.entries == nil {
		e.entries = make(map[string]cachedAvailabilityDecision)
	}
	if _, exists := e.entries[key]; !exists && len(e.entries) >= maxAvailabilityCacheEntries {
		e.evictOldestEntry()
	}
	e.entries[key] = cachedAvailabilityDecision{decision: decision, fetched: now}
	e.mu.Unlock()
	return decision, nil
}

func (e *CachedAvailabilityEvaluator) evictOldestEntry() {
	var oldestKey string
	var oldestFetched time.Time
	for key, entry := range e.entries {
		if oldestKey == "" || entry.fetched.Before(oldestFetched) {
			oldestKey = key
			oldestFetched = entry.fetched
		}
	}
	if oldestKey != "" {
		delete(e.entries, oldestKey)
	}
}

func availabilityCacheKey(evaluation EndpointAvailabilityEvaluation) string {
	return strings.Join([]string{
		strings.TrimSpace(evaluation.Phase),
		strings.TrimSpace(evaluation.OperationID),
		strings.TrimSpace(evaluation.Subject.Caller),
		strings.TrimSpace(evaluation.Subject.AppID),
		strings.TrimSpace(evaluation.Subject.OrganizationID),
	}, "\x00")
}
