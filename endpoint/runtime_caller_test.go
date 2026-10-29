package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	callerpkg "github.com/inttegro/httpapi/caller"
	requestpkg "github.com/inttegro/httpapi/request"
	responsepkg "github.com/inttegro/httpapi/response"
)

var (
	endpointPublicCaller    = callerpkg.Define("public-api")
	endpointWorkerCaller    = callerpkg.Define("worker")
	endpointDashboardCaller = callerpkg.Define("dashboard")
	endpointAdminCaller     = callerpkg.Define("admin")
)

func TestDefineEndpointCallerAvailability(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/orders/new",
		Handler: noopTranscriptionHandler,
		Access: EndpointAccessSpec{
			Callers: []callerpkg.Caller{endpointWorkerCaller, endpointDashboardCaller},
		},
	})

	if !endpoint.RestrictsCallers() {
		t.Fatal("endpoint does not restrict callers")
	}
	assertEndpointCallers(t, endpoint.AvailableCallers(), endpointWorkerCaller, endpointDashboardCaller)
}

func TestLegacyEndpointOptionCallerAvailability(t *testing.T) {
	endpoint := NewEndpoint(
		POST,
		"/orders/new",
		noopTranscriptionHandler,
		AvailableTo(endpointWorkerCaller),
	)

	assertEndpointCallers(t, endpoint.AvailableCallers(), endpointWorkerCaller)
}

func TestEndpointWithoutCallerAvailabilityAllowsAll(t *testing.T) {
	var called bool
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/orders/new",
		Handler: func(r *Req) {
			called = true
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]bool{"ok": true})
		},
	})

	if endpoint.RestrictsCallers() {
		t.Fatal("endpoint unexpectedly restricts callers")
	}
	if !endpoint.AvailableTo(endpointPublicCaller) {
		t.Fatal("endpoint without caller availability should allow public caller")
	}
	if !endpoint.AvailableTo(endpointWorkerCaller) {
		t.Fatal("endpoint without caller availability should allow worker caller")
	}

	req := httptest.NewRequest(POST, "/orders/new", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !called {
		t.Fatal("handler did not run for unrestricted endpoint")
	}
}

func TestEndpointGroupCallerAvailabilityNarrowsEndpointAvailability(t *testing.T) {
	group := EndpointGroup{PathPrefix: "/commerce"}
	group.AvailableTo(endpointWorkerCaller, endpointDashboardCaller)
	group.Add(DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/orders/new",
		Handler: noopTranscriptionHandler,
		Access: EndpointAccessSpec{
			Callers: []callerpkg.Caller{endpointWorkerCaller, endpointAdminCaller},
		},
	}))

	if !group.Endpoints[0].RestrictsCallers() {
		t.Fatal("endpoint does not restrict callers")
	}
	assertEndpointCallers(t, group.Endpoints[0].AvailableCallers(), endpointWorkerCaller)
}

func TestEndpointGroupWithoutCallerAvailabilityAllowsAll(t *testing.T) {
	group := EndpointGroup{PathPrefix: "/commerce"}
	group.Add(DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/orders/new",
		Handler: noopTranscriptionHandler,
	}))

	if group.Endpoints[0].RestrictsCallers() {
		t.Fatal("group without caller availability unexpectedly restricted endpoint")
	}
	if !group.Endpoints[0].AvailableTo(endpointPublicCaller) {
		t.Fatal("group without caller availability should allow public caller")
	}
	if !group.Endpoints[0].AvailableTo(endpointWorkerCaller) {
		t.Fatal("group without caller availability should allow worker caller")
	}
}

func TestEndpointGroupWithoutCallerAvailabilityDoesNotWidenEndpointRestriction(t *testing.T) {
	group := EndpointGroup{PathPrefix: "/commerce"}
	group.Add(DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/orders/new",
		Handler: noopTranscriptionHandler,
		Access: EndpointAccessSpec{
			Callers: []callerpkg.Caller{endpointWorkerCaller},
		},
	}))

	if !group.Endpoints[0].RestrictsCallers() {
		t.Fatal("endpoint caller restriction was lost")
	}
	if group.Endpoints[0].AvailableTo(endpointPublicCaller) {
		t.Fatal("unrestricted group widened endpoint caller restriction")
	}
	if !group.Endpoints[0].AvailableTo(endpointWorkerCaller) {
		t.Fatal("endpoint caller restriction should still allow worker caller")
	}
}

func TestEndpointGroupCallerAvailabilityCanDenyAll(t *testing.T) {
	group := EndpointGroup{PathPrefix: "/commerce"}
	group.AvailableTo(endpointPublicCaller)
	group.Add(DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/orders/new",
		Handler: noopTranscriptionHandler,
		Access: EndpointAccessSpec{
			Callers: []callerpkg.Caller{endpointAdminCaller},
		},
	}))

	if !group.Endpoints[0].RestrictsCallers() {
		t.Fatal("disjoint availability became unrestricted")
	}
	if len(group.Endpoints[0].AvailableCallers()) != 0 {
		t.Fatalf("available callers = %#v, want none", group.Endpoints[0].AvailableCallers())
	}
}

func TestEndpointHandlerRejectsUnavailableCaller(t *testing.T) {
	var called bool
	endpoint := callerRestrictedEndpoint(&called)
	req := httptest.NewRequest(POST, "/orders/new", strings.NewReader(`{}`)).
		WithContext(requestpkg.ContextWithCaller(context.Background(), endpointPublicCaller))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if called {
		t.Fatal("handler ran for unavailable caller")
	}
	if !strings.Contains(rec.Body.String(), endpointCallerDeniedCode) {
		t.Fatalf("body = %s, want %s", rec.Body.String(), endpointCallerDeniedCode)
	}
}

func TestEndpointHandlerRejectsMissingCaller(t *testing.T) {
	var called bool
	endpoint := callerRestrictedEndpoint(&called)
	req := httptest.NewRequest(POST, "/orders/new", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if called {
		t.Fatal("handler ran without a caller")
	}
	if !strings.Contains(rec.Body.String(), endpointCallerRequiredCode) {
		t.Fatalf("body = %s, want %s", rec.Body.String(), endpointCallerRequiredCode)
	}
}

func TestEndpointHandlerAllowsAvailableCaller(t *testing.T) {
	var called bool
	endpoint := callerRestrictedEndpoint(&called)
	req := httptest.NewRequest(POST, "/orders/new", strings.NewReader(`{}`)).
		WithContext(requestpkg.ContextWithCaller(context.Background(), endpointWorkerCaller))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !called {
		t.Fatal("handler did not run for available caller")
	}
}

func callerRestrictedEndpoint(called *bool) Endpoint {
	return DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/orders/new",
		Access: EndpointAccessSpec{
			Callers: []callerpkg.Caller{endpointWorkerCaller},
		},
		Handler: func(r *Req) {
			*called = true
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]bool{"ok": true})
		},
	})
}

func assertEndpointCallers(t *testing.T, got []callerpkg.Caller, want ...callerpkg.Caller) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("callers = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("callers = %#v, want %#v", got, want)
		}
	}
}
