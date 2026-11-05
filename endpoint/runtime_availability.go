package endpoint

import (
	"context"
	"strings"
	"sync"
	"time"

	errresp "github.com/inttegro/httpapi/erreur"
)

const endpointUnavailableCode = "endpoint_unavailable"

// EndpointAvailabilitySpec configures global availability for an endpoint.
type EndpointAvailabilitySpec struct {
	// Protected prevents the endpoint from being taken offline through the
	// global control plane. Use it for health and control-plane routes.
	Protected bool `json:"protected,omitempty" yaml:"protected,omitempty"`
}

type endpointAvailabilityPolicy struct {
	protected bool
}

// EndpointAvailabilityEvaluation identifies the operation whose global
// availability must be checked before its request body or handler is run.
// OperationID is the stable control-plane key; aliases therefore share the
// canonical endpoint's availability state.
type EndpointAvailabilityEvaluation struct {
	OperationID string
	Method      HttpMethod
	Pattern     string
	RequestPath string
}

// EndpointAvailabilityDecision is an audit-safe global availability decision.
// An unavailable decision remains in force until the evaluator reports the
// operation available again or ExpiresAt passes.
type EndpointAvailabilityDecision struct {
	Available bool
	Provider  string
	Reason    string
	ExpiresAt time.Time
}

// AvailabilityEvaluator decides whether an endpoint operation is globally
// available. It is intentionally separate from per-app endpoint controls.
type AvailabilityEvaluator interface {
	EvaluateEndpointAvailability(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error)
}

// AvailabilityEvaluatorFunc adapts a function to AvailabilityEvaluator.
type AvailabilityEvaluatorFunc func(
	context.Context,
	EndpointAvailabilityEvaluation,
) (EndpointAvailabilityDecision, error)

// EvaluateEndpointAvailability calls f(ctx, evaluation).
func (f AvailabilityEvaluatorFunc) EvaluateEndpointAvailability(
	ctx context.Context,
	evaluation EndpointAvailabilityEvaluation,
) (EndpointAvailabilityDecision, error) {
	return f(ctx, evaluation)
}

type availableEndpointEvaluator struct{}

func (availableEndpointEvaluator) EvaluateEndpointAvailability(
	context.Context,
	EndpointAvailabilityEvaluation,
) (EndpointAvailabilityDecision, error) {
	return EndpointAvailabilityDecision{Available: true, Provider: "default"}, nil
}

var (
	availabilityEvaluatorMu sync.RWMutex
	availabilityEvaluator   AvailabilityEvaluator = availableEndpointEvaluator{}
)

// ConfigureAvailabilityEvaluator installs the package-level global endpoint
// availability evaluator. Unconfigured services allow all endpoints. Evaluator
// errors also fail open so a control-plane outage cannot take the data plane
// offline; service adapters should retain stale decisions on storage errors.
//
// The returned function restores the previous evaluator for tests and
// short-lived overrides.
func ConfigureAvailabilityEvaluator(evaluator AvailabilityEvaluator) func() {
	if evaluator == nil {
		evaluator = availableEndpointEvaluator{}
	}

	availabilityEvaluatorMu.Lock()
	prev := availabilityEvaluator
	availabilityEvaluator = evaluator
	availabilityEvaluatorMu.Unlock()

	return func() {
		availabilityEvaluatorMu.Lock()
		availabilityEvaluator = prev
		availabilityEvaluatorMu.Unlock()
	}
}

func currentAvailabilityEvaluator() AvailabilityEvaluator {
	availabilityEvaluatorMu.RLock()
	defer availabilityEvaluatorMu.RUnlock()
	return availabilityEvaluator
}

func (e Endpoint) availabilityError(r *Req) *errresp.Error {
	operationID := strings.TrimSpace(e.Operation().ID)
	if operationID == "" || !e.GloballyControllable() {
		return nil
	}

	evaluation := EndpointAvailabilityEvaluation{
		OperationID: operationID,
		Method:      e.Method(),
		Pattern:     e.Pattern(),
	}
	ctx := context.Background()
	if r != nil && r.Req != nil {
		ctx = r.Req.Context()
		evaluation.RequestPath = r.Req.URL.Path
	}

	decision, err := currentAvailabilityEvaluator().EvaluateEndpointAvailability(ctx, evaluation)
	if err != nil {
		logr.Printf(
			"endpoint availability evaluation failed:"+
				" request_id=%s operation_id=%s method=%s path=%s error=%v",
			requestID(r), operationID, evaluation.Method, evaluation.RequestPath, err,
		)
		return nil
	}
	if decision.Available || (!decision.ExpiresAt.IsZero() && !time.Now().Before(decision.ExpiresAt)) {
		return nil
	}

	return errresp.Transient(
		endpointUnavailableCode,
		"endpoint is temporarily unavailable",
		"this API operation is temporarily offline. retry later or contact support if the problem persists.",
	)
}

// GloballyControllable reports whether the endpoint may be taken offline by
// its stable operation ID.
func (e Endpoint) GloballyControllable() bool {
	return strings.TrimSpace(e.Operation().ID) != "" && !e.availability.protected
}

// AvailabilitySpec returns the endpoint's global availability metadata.
func (e Endpoint) AvailabilitySpec() EndpointAvailabilitySpec {
	return EndpointAvailabilitySpec{Protected: e.availability.protected}
}
