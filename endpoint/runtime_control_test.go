package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	requestpkg "github.com/zebodotdev/httpapi/request"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

func TestEndpointControlDeniesWhenEvaluatorNotConfigured(t *testing.T) {
	restore := ConfigureControlEvaluator(nil)
	defer restore()

	var called bool
	endpoint := controlledTestEndpoint(&called)
	req := controlledTestRequest()
	rec := httptest.NewRecorder()

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if called {
		t.Fatal("handler ran without control evaluator")
	}
	if !strings.Contains(rec.Body.String(), endpointControlUnavailableCode) {
		t.Fatalf("body = %s, want %s", rec.Body.String(), endpointControlUnavailableCode)
	}
}

func TestEndpointControlDeniesRejectedDecision(t *testing.T) {
	restore := ConfigureControlEvaluator(ControlEvaluatorFunc(func(
		context.Context,
		EndpointControlEvaluation,
	) (EndpointControlDecision, error) {
		return EndpointControlDecision{Allowed: false, Provider: "test"}, nil
	}))
	defer restore()

	var called bool
	endpoint := controlledTestEndpoint(&called)
	req := controlledTestRequest()
	rec := httptest.NewRecorder()

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if called {
		t.Fatal("handler ran after denied control decision")
	}
	if !strings.Contains(rec.Body.String(), endpointControlDeniedCode) {
		t.Fatalf("body = %s, want %s", rec.Body.String(), endpointControlDeniedCode)
	}
}

func TestEndpointControlAllowsAcceptedDecision(t *testing.T) {
	restore := ConfigureControlEvaluator(ControlEvaluatorFunc(func(
		context.Context,
		EndpointControlEvaluation,
	) (EndpointControlDecision, error) {
		return EndpointControlDecision{Allowed: true, Provider: "test"}, nil
	}))
	defer restore()

	var called bool
	endpoint := controlledTestEndpoint(&called)
	req := controlledTestRequest()
	rec := httptest.NewRecorder()

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !called {
		t.Fatal("handler did not run after allowed control decision")
	}
}

func controlledTestEndpoint(called *bool) Endpoint {
	return DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/collection_requests/new",
		Handler: func(r *Req) {
			*called = true
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]bool{"ok": true})
		},
		Access: EndpointAccessSpec{
			Authorization: RequiredAuthorization(AuthorizationKindAny),
		},
		Controls: []EndpointControlSpec{{
			Kind: EndpointControlKindGate,
			Key:  "merx_private_beta",
		}},
	})
}

func controlledTestRequest() *http.Request {
	req := httptest.NewRequest(POST, "/collection_requests/new", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	return req.WithContext(requestpkg.ContextWithAuthenticatedApp(req.Context(), "app_123"))
}
