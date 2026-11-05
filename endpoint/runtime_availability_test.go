package endpoint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	callerpkg "github.com/inttegro/httpapi/caller"
	requestpkg "github.com/inttegro/httpapi/request"
	responsepkg "github.com/inttegro/httpapi/response"
)

func TestEndpointAvailabilityBlocksBeforeHandler(t *testing.T) {
	var evaluation EndpointAvailabilityEvaluation
	restore := ConfigureAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		_ context.Context,
		input EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		evaluation = input
		return EndpointAvailabilityDecision{
			Available: false,
			Provider:  "test",
			Reason:    "maintenance in progress",
		}, nil
	}))
	defer restore()

	var called bool
	endpoint := availabilityTestEndpoint(&called, "createWidget")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(POST, "/widgets", strings.NewReader(`{"name":"slow"}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if called {
		t.Fatal("handler ran for an offline endpoint")
	}
	if !strings.Contains(rec.Body.String(), endpointUnavailableCode) {
		t.Fatalf("body = %s, want %s", rec.Body.String(), endpointUnavailableCode)
	}
	if strings.Contains(rec.Body.String(), "maintenance in progress") {
		t.Fatalf("body leaked the operator-only availability reason: %s", rec.Body.String())
	}
	if evaluation.OperationID != "createWidget" || evaluation.RequestPath != "/widgets" {
		t.Fatalf("evaluation = %#v", evaluation)
	}
}

func TestEndpointAvailabilityAllowsExpiredOfflineDecision(t *testing.T) {
	restore := ConfigureAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		return EndpointAvailabilityDecision{
			Available: false,
			ExpiresAt: time.Now().Add(-time.Second),
		}, nil
	}))
	defer restore()

	var called bool
	rec := httptest.NewRecorder()
	availabilityTestEndpoint(&called, "createWidget").Handler()(
		rec,
		httptest.NewRequest(POST, "/widgets", strings.NewReader(`{}`)),
	)

	if rec.Code != http.StatusAccepted || !called {
		t.Fatalf("status = %d called = %t; body=%s", rec.Code, called, rec.Body.String())
	}
}

func TestEndpointAvailabilityEvaluatorFailureFailsOpen(t *testing.T) {
	restore := ConfigureAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		return EndpointAvailabilityDecision{}, errors.New("storage unavailable")
	}))
	defer restore()

	var called bool
	rec := httptest.NewRecorder()
	availabilityTestEndpoint(&called, "createWidget").Handler()(
		rec,
		httptest.NewRequest(POST, "/widgets", strings.NewReader(`{}`)),
	)

	if rec.Code != http.StatusAccepted || !called {
		t.Fatalf("status = %d called = %t; body=%s", rec.Code, called, rec.Body.String())
	}
}

func TestEndpointAvailabilityBlocksAuthenticatedScopedSubject(t *testing.T) {
	var evaluations []EndpointAvailabilityEvaluation
	restore := ConfigureAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		_ context.Context,
		input EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		evaluations = append(evaluations, input)
		return EndpointAvailabilityDecision{
			Available: input.Phase != EndpointAvailabilityPhaseScoped || input.Subject.AppID != "app_blocked",
			Provider:  "test",
		}, nil
	}))
	defer restore()

	var called bool
	req := httptest.NewRequest(POST, "/widgets", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	req = req.WithContext(requestpkg.ContextWithCaller(
		requestpkg.ContextWithSession(req.Context(), &requestpkg.Session{
			ID:             "session_123",
			App:            requestpkg.App{ID: "app_blocked"},
			OrganizationID: "org_123",
			InitiatedAt:    time.Now().Add(-time.Minute),
			ExpiresAt:      time.Now().Add(time.Minute),
			MultiUse:       true,
			AuthMode:       requestpkg.SessionAuthModeBearer,
		}),
		callerpkg.Define("dashboard"),
	))
	rec := httptest.NewRecorder()
	availabilityTestEndpoint(&called, "createWidget").Handler()(rec, req)

	if rec.Code != http.StatusServiceUnavailable || called {
		t.Fatalf("status = %d called = %t; body=%s", rec.Code, called, rec.Body.String())
	}
	if len(evaluations) != 2 {
		t.Fatalf("evaluations = %#v, want global and scoped", evaluations)
	}
	scoped := evaluations[1]
	if scoped.Phase != EndpointAvailabilityPhaseScoped ||
		scoped.Subject.Caller != "dashboard" ||
		scoped.Subject.AppID != "app_blocked" ||
		scoped.Subject.OrganizationID != "org_123" {
		t.Fatalf("scoped evaluation = %#v", scoped)
	}
}

func TestEndpointWithoutOperationIDIsNotGloballyControllable(t *testing.T) {
	restore := ConfigureAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		t.Fatal("availability evaluator called without an operation id")
		return EndpointAvailabilityDecision{}, nil
	}))
	defer restore()

	var called bool
	rec := httptest.NewRecorder()
	availabilityTestEndpoint(&called, "").Handler()(
		rec,
		httptest.NewRequest(POST, "/widgets", strings.NewReader(`{}`)),
	)

	if rec.Code != http.StatusAccepted || !called {
		t.Fatalf("status = %d called = %t; body=%s", rec.Code, called, rec.Body.String())
	}
}

func TestProtectedEndpointIsNotGloballyControllable(t *testing.T) {
	restore := ConfigureAvailabilityEvaluator(AvailabilityEvaluatorFunc(func(
		context.Context,
		EndpointAvailabilityEvaluation,
	) (EndpointAvailabilityDecision, error) {
		t.Fatal("availability evaluator called for a protected endpoint")
		return EndpointAvailabilityDecision{}, nil
	}))
	defer restore()

	var called bool
	protected := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/health",
		Handler: func(r *Req) {
			called = true
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]bool{"ok": true})
		},
		Operation:    OperationSpec{ID: "health", Summary: "Health"},
		Availability: EndpointAvailabilitySpec{Protected: true},
	})
	rec := httptest.NewRecorder()
	protected.Handler()(rec, httptest.NewRequest(POST, "/health", strings.NewReader(`{}`)))

	if rec.Code != http.StatusAccepted || !called || protected.GloballyControllable() {
		t.Fatalf("status = %d called = %t controllable = %t", rec.Code, called, protected.GloballyControllable())
	}
}

func availabilityTestEndpoint(called *bool, operationID string) Endpoint {
	return DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/widgets",
		Handler: func(r *Req) {
			*called = true
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]bool{"ok": true})
		},
		Operation: OperationSpec{ID: operationID, Summary: "Create a widget"},
	})
}
