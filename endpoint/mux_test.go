package endpoint

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	responsepkg "github.com/zebodotdev/httpapi/response"
)

func TestMuxMountServesEndpointAndRecordsMetadata(t *testing.T) {
	mux := NewMux()
	group := EndpointGroup{
		PathPrefix: "/v1",
		Endpoints: []Endpoint{
			jsonEndpoint(GET, "/health", map[string]bool{"ok": true}),
		},
	}

	if err := mux.Mount(group); err != nil {
		t.Fatalf("mount group: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(GET, "/v1/health", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want ok response", rec.Body.String())
	}

	mounted := mux.MountedEndpoints()
	if len(mounted) != 1 {
		t.Fatalf("mounted endpoints = %d, want 1", len(mounted))
	}
	if mounted[0].Method != GET || mounted[0].Path != "/v1/health" {
		t.Fatalf("mounted endpoint = %#v", mounted[0])
	}
	if mounted[0].Endpoint.Pattern() != "/health" {
		t.Fatalf("endpoint pattern = %q, want /health", mounted[0].Endpoint.Pattern())
	}
}

func TestMuxMountAppliesGroupDefaults(t *testing.T) {
	mux := NewMux()
	group := EndpointGroup{
		PathPrefix: "/internal",
		Auth:       RequiredAuthorization(AuthorizationKindService),
		Priority:   EndpointPriorityCritical,
		Timeout: EndpointTimeoutSpec{
			Handler: time.Second,
		},
		Limits: EndpointLimitsSpec{
			MaxRequestBytes: 2048,
		},
		Endpoints: []Endpoint{
			jsonEndpoint(POST, "sync", map[string]bool{"ok": true}),
		},
	}

	if err := mux.Mount(group); err != nil {
		t.Fatalf("mount group: %v", err)
	}

	mounted := mux.MountedEndpoints()
	if len(mounted) != 1 {
		t.Fatalf("mounted endpoints = %d, want 1", len(mounted))
	}
	endpoint := mounted[0].Endpoint
	if mounted[0].Path != "/internal/sync" {
		t.Fatalf("mounted path = %q, want /internal/sync", mounted[0].Path)
	}
	if got, want := endpoint.Authorization(), group.Authorization(); got != want {
		t.Fatalf("authorization = %#v, want %#v", got, want)
	}
	if got, want := endpoint.Priority(), EndpointPriorityCritical; got != want {
		t.Fatalf("priority = %q, want %q", got, want)
	}
	if got, want := endpoint.TimeoutSpec().Handler, time.Second; got != want {
		t.Fatalf("handler timeout = %s, want %s", got, want)
	}
	if got, want := endpoint.LimitsSpec().MaxRequestBytes, int64(2048); got != want {
		t.Fatalf("max request bytes = %d, want %d", got, want)
	}
}

func TestMuxMountServesEndpointAliasesAndRecordsMetadata(t *testing.T) {
	mux := NewMux()
	group := EndpointGroup{
		PathPrefix: "/orders",
		Endpoints: []Endpoint{
			DefineEndpoint(EndpointSpec{
				Method: GET,
				Path:   "/create",
				Aliases: []AliasSpec{
					{Path: "/new"},
				},
				Handler: func(r *Req) {
					responsepkg.RenderJSON(r, http.StatusOK, map[string]string{
						"path": r.Path(),
					})
				},
			}),
		},
	}

	if err := mux.Mount(group); err != nil {
		t.Fatalf("mount group: %v", err)
	}

	for _, path := range []string{"/orders/create", "/orders/new"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(GET, path, nil)
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200; body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"path":"`+path+`"`) {
			t.Fatalf("%s body = %s, want matched path", path, rec.Body.String())
		}
	}

	mounted := mux.MountedEndpoints()
	if len(mounted) != 2 {
		t.Fatalf("mounted endpoints = %d, want 2", len(mounted))
	}
	if mounted[0].Path != "/orders/create" || mounted[0].Alias != nil {
		t.Fatalf("canonical mounted endpoint = %#v", mounted[0])
	}
	if mounted[1].Path != "/orders/new" || mounted[1].Alias == nil {
		t.Fatalf("alias mounted endpoint = %#v", mounted[1])
	}
	if mounted[1].Alias.Path != "/new" {
		t.Fatalf("alias spec path = %q, want /new", mounted[1].Alias.Path)
	}
	if mounted[1].Endpoint.Pattern() != "/create" {
		t.Fatalf("alias endpoint pattern = %q, want /create", mounted[1].Endpoint.Pattern())
	}

	mounted[1].Alias.Path = "/mutated"
	if got := mux.MountedEndpoints(); got[1].Alias.Path != "/new" {
		t.Fatalf("mounted alias accessor exposed mutable state: %#v", got[1].Alias)
	}
}

func TestMuxMountRejectsDuplicateWithinBatchWithoutRegistering(t *testing.T) {
	mux := NewMux()
	group := EndpointGroup{
		PathPrefix: "/v1",
		Endpoints: []Endpoint{
			jsonEndpoint(GET, "/health", map[string]int{"version": 1}),
			jsonEndpoint(GET, "/health", map[string]int{"version": 2}),
		},
	}

	err := mux.Mount(group)
	if err == nil {
		t.Fatal("expected duplicate mount error")
	}
	if _, ok := errors.AsType[ErrDuplicateMuxRoute](err); !ok {
		t.Fatalf("error = %T %v, want ErrDuplicateMuxRoute", err, err)
	}
	if len(mux.MountedEndpoints()) != 0 {
		t.Fatalf("mounted endpoints = %d, want 0", len(mux.MountedEndpoints()))
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(GET, "/v1/health", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 after rejected batch", rec.Code)
	}
}

func TestMuxMountRejectsAliasRouteCollisionWithoutRegistering(t *testing.T) {
	mux := NewMux()
	group := EndpointGroup{
		PathPrefix: "/orders",
		Endpoints: []Endpoint{
			DefineEndpoint(EndpointSpec{
				Method:  GET,
				Path:    "/create",
				Aliases: []AliasSpec{{Path: "/new"}},
				Handler: noopTranscriptionHandler,
			}),
			jsonEndpoint(GET, "/new", map[string]int{"version": 2}),
		},
	}

	err := mux.Mount(group)
	if err == nil {
		t.Fatal("expected duplicate mount error")
	}
	var duplicate ErrDuplicateMuxRoute
	if !errors.As(err, &duplicate) {
		t.Fatalf("error = %T %v, want ErrDuplicateMuxRoute", err, err)
	}
	if duplicate.Path != "/orders/new" || duplicate.Method != GET {
		t.Fatalf("duplicate route = %#v, want GET /orders/new", duplicate)
	}
	if len(mux.MountedEndpoints()) != 0 {
		t.Fatalf("mounted endpoints = %d, want 0", len(mux.MountedEndpoints()))
	}
}

func TestMuxMountRejectsDuplicateAcrossCalls(t *testing.T) {
	mux := NewMux()
	group := EndpointGroup{
		PathPrefix: "/v1",
		Endpoints: []Endpoint{
			jsonEndpoint(GET, "/health", map[string]int{"version": 1}),
		},
	}
	if err := mux.Mount(group); err != nil {
		t.Fatalf("mount first group: %v", err)
	}

	err := mux.Mount(group)
	if err == nil {
		t.Fatal("expected duplicate mount error")
	}
	if len(mux.MountedEndpoints()) != 1 {
		t.Fatalf("mounted endpoints = %d, want original endpoint only", len(mux.MountedEndpoints()))
	}
}

func TestMuxCanUseCallerProvidedServeMux(t *testing.T) {
	stdlibMux := http.NewServeMux()
	mux := NewMux(WithServeMux(stdlibMux))
	mux.MustMount(EndpointGroup{
		PathPrefix: "/v1",
		Endpoints: []Endpoint{
			jsonEndpoint(GET, "/ready", map[string]bool{"ready": true}),
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(GET, "/v1/ready", nil)
	stdlibMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ready":true`) {
		t.Fatalf("body = %s, want ready response", rec.Body.String())
	}
}

func TestEndpointGroupMountDelegatesToMux(t *testing.T) {
	stdlibMux := http.NewServeMux()
	group := EndpointGroup{
		PathPrefix: "/v1",
		Endpoints: []Endpoint{
			jsonEndpoint(GET, "/legacy", map[string]bool{"ok": true}),
		},
	}

	group.Mount(stdlibMux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(GET, "/v1/legacy", nil)
	stdlibMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEndpointGroupMountRejectsNilServeMux(t *testing.T) {
	defer func() {
		if recovered := recover(); !errors.Is(recoveredAsError(recovered), ErrNilServeMux) {
			t.Fatalf("panic = %v, want ErrNilServeMux", recovered)
		}
	}()

	group := EndpointGroup{
		Endpoints: []Endpoint{
			jsonEndpoint(GET, "/legacy", map[string]bool{"ok": true}),
		},
	}
	group.Mount(nil)
}

func TestMuxMountEndpointsRegistersUngroupedEndpoints(t *testing.T) {
	mux := NewMux()
	if err := mux.MountEndpoints(jsonEndpoint(GET, "/status", "ok")); err != nil {
		t.Fatalf("mount endpoint: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(GET, "/status", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestMuxMountNilReceiverFails(t *testing.T) {
	var mux *Mux
	if err := mux.Mount(EndpointGroup{}); !errors.Is(err, ErrNilMux) {
		t.Fatalf("error = %v, want ErrNilMux", err)
	}
}

func jsonEndpoint(method HttpMethod, path string, body any) Endpoint {
	return DefineEndpoint(EndpointSpec{
		Method: method,
		Path:   path,
		Handler: func(r *Req) {
			responsepkg.RenderJSON(r, http.StatusOK, body)
		},
	})
}

func recoveredAsError(recovered any) error {
	err, _ := recovered.(error)
	return err
}
