package cost

import (
	"context"
	"strings"
	"time"
)

// OperationSink records operation usage events at a service-owned durable
// boundary.
//
// httpapi only defines the event shape. OperationSink implementations own
// pricing, storage, retry behavior, calibration, invoice matching, and
// reconciliation. A typical service sink prices each UsageUnit with
// service-local policy, persists an estimate keyed by Operation.ID/RootID, and
// later reconciles that estimate against provider invoices or usage exports.
type OperationSink interface {
	RecordOperation(context.Context, OperationEvent) error
}

// OperationSinkFunc adapts a function to OperationSink.
type OperationSinkFunc func(context.Context, OperationEvent) error

// RecordOperation calls f(ctx, event).
func (f OperationSinkFunc) RecordOperation(ctx context.Context, event OperationEvent) error {
	if f == nil {
		return nil
	}
	return f(ctx, event.Clone())
}

type noopOperationSink struct{}

// NoopOperationSink returns a sink that ignores every operation event.
func NoopOperationSink() OperationSink {
	return noopOperationSink{}
}

// RecordOperation ignores event.
func (noopOperationSink) RecordOperation(context.Context, OperationEvent) error {
	return nil
}

// Estimator converts provider-neutral operation usage into a service-owned
// estimate.
//
// Implementations may use pricing policy, provider price lists, discounts, or
// calibration data outside httpapi. httpapi does not provide an implementation.
type Estimator interface {
	EstimateOperation(context.Context, OperationEvent) (EstimateEvent, error)
}

// EstimatorFunc adapts a function to Estimator.
type EstimatorFunc func(context.Context, OperationEvent) (EstimateEvent, error)

// EstimateOperation calls f(ctx, event).
func (f EstimatorFunc) EstimateOperation(
	ctx context.Context,
	event OperationEvent,
) (EstimateEvent, error) {
	if f == nil {
		return EstimateEvent{}, nil
	}
	return f(ctx, event.Clone())
}

// EstimateSink records service-owned estimate events at a durable boundary.
type EstimateSink interface {
	RecordEstimate(context.Context, EstimateEvent) error
}

// EstimateSinkFunc adapts a function to EstimateSink.
type EstimateSinkFunc func(context.Context, EstimateEvent) error

// RecordEstimate calls f(ctx, estimate).
func (f EstimateSinkFunc) RecordEstimate(ctx context.Context, estimate EstimateEvent) error {
	if f == nil {
		return nil
	}
	return f(ctx, estimate.Clone())
}

type noopEstimateSink struct{}

// NoopEstimateSink returns a sink that ignores every estimate event.
func NoopEstimateSink() EstimateSink {
	return noopEstimateSink{}
}

// RecordEstimate ignores estimate.
func (noopEstimateSink) RecordEstimate(context.Context, EstimateEvent) error {
	return nil
}

// EstimateEvent is the service-owned estimate output for one operation event.
//
// Currency and Amount are allowed here because this type sits on the downstream
// side of httpapi's neutral usage boundary. httpapi does not decide how the
// amount is calculated, stored, presented, calibrated, or reconciled.
type EstimateEvent struct {
	// Operation identifies the operation being estimated.
	Operation Operation `json:"operation"`

	// Request describes the completed request when the operation came from HTTP.
	Request RequestMetadata `json:"request"`

	// Endpoint describes the endpoint when the operation came from HTTP.
	Endpoint EndpointMetadata `json:"endpoint"`

	// Usage contains the provider-neutral usage included in this estimate.
	Usage []UsageUnit `json:"usage,omitempty"`

	// Currency is the ISO 4217 currency code selected by service pricing policy.
	Currency string `json:"currency,omitempty"`

	// Amount is the exact decimal estimate in Currency.
	Amount Quantity `json:"amount"`

	// EstimatedAt is when the estimate was produced.
	EstimatedAt time.Time `json:"estimated_at"`

	// Labels carries optional low-cardinality service-owned estimate metadata.
	Labels map[string]string `json:"labels,omitempty"`
}

// NewEstimateEvent returns an estimate event for operation event.
func NewEstimateEvent(event OperationEvent, currency string, amount Quantity) EstimateEvent {
	return EstimateEvent{
		Operation:   event.Operation,
		Request:     event.Request,
		Endpoint:    event.Endpoint,
		Usage:       cloneUsageUnits(event.Usage),
		Currency:    strings.TrimSpace(strings.ToUpper(currency)),
		Amount:      amount,
		EstimatedAt: time.Now(),
	}
}

// WithLabel returns a copy of e with name set to value in Labels.
func (e EstimateEvent) WithLabel(name, value string) EstimateEvent {
	if e.Labels == nil {
		e.Labels = map[string]string{}
	} else {
		e.Labels = cloneLabels(e.Labels)
	}
	e.Labels[name] = value
	return e
}

// Clone returns a copy of e with cloned usage and labels.
func (e EstimateEvent) Clone() EstimateEvent {
	e.Usage = cloneUsageUnits(e.Usage)
	e.Labels = cloneLabels(e.Labels)
	return e
}
