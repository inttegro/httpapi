package endpoint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	errresp "github.com/zebodotdev/httpapi/erreur"
)

const (
	EndpointControlKindFeatureFlag    = "feature_flag"
	EndpointControlKindGate           = "gate"
	EndpointControlKindCircuitBreaker = "circuit_breaker"

	endpointControlAuthenticationRequiredCode = "endpoint_control_authentication_required"
	endpointControlDeniedCode                 = "endpoint_control_denied"
	endpointControlUnavailableCode            = "endpoint_control_unavailable"
)

// ErrControlEvaluatorNotConfigured is returned when a controlled endpoint runs
// before the service installs a concrete control evaluator.
var ErrControlEvaluatorNotConfigured = errors.New("httpapi: control_evaluator_not_configured")

// EndpointControlSpec declares a control that must allow the active request
// before the endpoint handler can run.
type EndpointControlSpec struct {
	Kind string
	Key  string
}

// EndpointControlEvaluation is passed to a service-owned control evaluator.
type EndpointControlEvaluation struct {
	Request *Req
	Control EndpointControlSpec
}

// EndpointControlDecision is an audit-safe control decision.
type EndpointControlDecision struct {
	Allowed    bool
	Assumed    bool
	Provider   string
	Reason     string
	Subject    string
	SubjectID  string
	Enrollment string
}

// ControlEvaluator decides whether a request satisfies an endpoint control.
type ControlEvaluator interface {
	EvaluateEndpointControl(context.Context, EndpointControlEvaluation) (EndpointControlDecision, error)
}

// ControlEvaluatorFunc adapts a function to ControlEvaluator.
type ControlEvaluatorFunc func(context.Context, EndpointControlEvaluation) (EndpointControlDecision, error)

// EvaluateEndpointControl calls f(ctx, evaluation).
func (f ControlEvaluatorFunc) EvaluateEndpointControl(
	ctx context.Context,
	evaluation EndpointControlEvaluation,
) (EndpointControlDecision, error) {
	return f(ctx, evaluation)
}

type unavailableControlEvaluator struct{}

func (unavailableControlEvaluator) EvaluateEndpointControl(
	context.Context,
	EndpointControlEvaluation,
) (EndpointControlDecision, error) {
	return EndpointControlDecision{}, ErrControlEvaluatorNotConfigured
}

type endpointControlPolicy struct {
	controls []EndpointControlSpec
}

var (
	controlEvaluatorMu sync.RWMutex
	controlEvaluator   ControlEvaluator = unavailableControlEvaluator{}
)

// ConfigureControlEvaluator installs the package-level control evaluator.
//
// Controlled endpoints fail closed until a service installs an evaluator. The
// returned function restores the previous evaluator for tests and short-lived
// overrides.
func ConfigureControlEvaluator(evaluator ControlEvaluator) func() {
	if evaluator == nil {
		evaluator = unavailableControlEvaluator{}
	}

	controlEvaluatorMu.Lock()
	prev := controlEvaluator
	controlEvaluator = evaluator
	controlEvaluatorMu.Unlock()

	return func() {
		controlEvaluatorMu.Lock()
		controlEvaluator = prev
		controlEvaluatorMu.Unlock()
	}
}

func currentControlEvaluator() ControlEvaluator {
	controlEvaluatorMu.RLock()
	defer controlEvaluatorMu.RUnlock()
	return controlEvaluator
}

func (e Endpoint) Controls() []EndpointControlSpec {
	return cloneEndpointControlSpecs(e.controlPolicy().controls)
}

func (e Endpoint) RequiresControls() bool {
	return len(e.controlPolicy().controls) > 0
}

func (e Endpoint) controlPolicy() endpointControlPolicy {
	return e.controls
}

func (e *Endpoint) mutableControlPolicy() *endpointControlPolicy {
	return &e.controls
}

func (e Endpoint) controlAccessError(r *Req) *errresp.Error {
	controls := e.Controls()
	if len(controls) == 0 {
		return nil
	}

	if r == nil || r.Sess == nil || !r.Authorized() {
		err := errresp.Unauthenticated(
			endpointControlAuthenticationRequiredCode,
			"endpoint control requires authentication",
			"this endpoint requires an authenticated app or organization before control policy can be evaluated.",
		)
		recordEndpointAccessFailure(r, err)
		return err
	}

	for _, control := range controls {
		if err := e.evaluateControlAccess(r, control); err != nil {
			return err
		}
	}

	return nil
}

func (e Endpoint) evaluateControlAccess(r *Req, control EndpointControlSpec) *errresp.Error {
	ctx := context.Background()
	if r != nil && r.Req != nil {
		ctx = r.Req.Context()
	}

	decision, evalErr := currentControlEvaluator().EvaluateEndpointControl(ctx, EndpointControlEvaluation{
		Request: r,
		Control: control,
	})
	if evalErr != nil {
		logr.Printf(
			"endpoint control evaluation failed:"+
				" request_id=%s control_kind=%s control_key=%s error=%v",
			requestID(r), control.Kind, control.Key, evalErr,
		)
		err := errresp.Forbidden(
			endpointControlUnavailableCode,
			"endpoint control could not be confirmed",
			"this endpoint requires an active control enrollment that could not be confirmed.",
		)
		recordEndpointAccessFailure(r, err)
		return err
	}

	if decision.Allowed {
		return nil
	}

	err := errresp.Forbidden(
		endpointControlDeniedCode,
		"endpoint control denied",
		fmt.Sprintf("this endpoint requires %s control %q.", control.Kind, control.Key),
	)
	recordEndpointAccessFailure(r, err)
	return err
}

func requestID(r *Req) string {
	if r == nil {
		return ""
	}
	return r.ID
}

func normalizeEndpointControlSpecs(controls []EndpointControlSpec) []EndpointControlSpec {
	if len(controls) == 0 {
		return nil
	}

	seen := map[string]bool{}
	normalized := make([]EndpointControlSpec, 0, len(controls))
	for _, control := range controls {
		control = normalizeEndpointControlSpec(control)
		if control.Kind == "" || control.Key == "" {
			continue
		}
		key := control.Kind + "\x00" + control.Key
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, control)
	}
	return normalized
}

func normalizeEndpointControlSpec(control EndpointControlSpec) EndpointControlSpec {
	return EndpointControlSpec{
		Kind: strings.TrimSpace(control.Kind),
		Key:  strings.TrimSpace(control.Key),
	}
}

func cloneEndpointControlSpecs(controls []EndpointControlSpec) []EndpointControlSpec {
	if len(controls) == 0 {
		return nil
	}
	return append([]EndpointControlSpec(nil), controls...)
}
