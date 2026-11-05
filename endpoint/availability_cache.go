package endpoint

import (
	"context"
	"sync"
	"time"
)

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

// NewCachedAvailabilityEvaluator wraps evaluator with a per-operation cache.
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
	key := evaluation.OperationID

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
	e.entries[key] = cachedAvailabilityDecision{decision: decision, fetched: now}
	e.mu.Unlock()
	return decision, nil
}
