package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	responsepkg "github.com/zebodotdev/httpapi/response"
)

func TestConfigCORSHandlesPreflightBeforeMiddleware(t *testing.T) {
	called := false
	handler := Handler(Config{
		CORS: PermissiveCORS(),
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			called = true
		}),
		Middleware: []Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Fatal("preflight entered configured middleware")
				})
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/tasks", nil)
	req.Header.Set("origin", "https://dashboard.example")
	req.Header.Set("access-control-request-method", http.MethodPost)
	req.Header.Set("access-control-request-headers", "authorization, content-type")

	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("preflight reached next handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body length = %d, want 0", rec.Body.Len())
	}
	assertHeader(t, rec.Header(), accessControlAllowOriginHeader, wildcardHeaderValue)
	assertHeader(t, rec.Header(), accessControlAllowMethodsHeader, wildcardHeaderValue)
	assertHeader(t, rec.Header(), accessControlAllowHeadersHeader, wildcardHeaderValue)
	assertHeaderContains(t, rec.Header(), varyHeader, originHeader)
	assertHeaderContains(t, rec.Header(), varyHeader, accessControlRequestMethodHeader)
	assertHeaderContains(t, rec.Header(), varyHeader, accessControlRequestHeadersHeader)
}

func TestCORSActualRequestEntersMiddlewareAndHandler(t *testing.T) {
	events := []string{}
	handler := Handler(Config{
		CORS: PermissiveCORS(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			events = append(events, "handler")
			w.WriteHeader(http.StatusAccepted)
		}),
		Middleware: []Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					events = append(events, "middleware")
					next.ServeHTTP(w, r)
				})
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	req.Header.Set("origin", "https://dashboard.example")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if len(events) != 2 || events[0] != "middleware" || events[1] != "handler" {
		t.Fatalf("events = %#v", events)
	}
	assertHeader(t, rec.Header(), accessControlAllowOriginHeader, wildcardHeaderValue)
	assertHeader(
		t,
		rec.Header(),
		accessControlExposeHeadersHeader,
		responsepkg.HeaderRequestID+", "+responsepkg.HeaderRetryAfter,
	)
}

func TestCORSPlainOptionsPassesThrough(t *testing.T) {
	handler := CORSMiddleware(*PermissiveCORS())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/tasks", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if got := rec.Header().Get(accessControlAllowOriginHeader); got != "" {
		t.Fatalf("allow origin = %q, want empty", got)
	}
}

func TestCORSStrictPolicyAllowsPreflight(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{
		AllowedOrigins: []string{"https://dashboard.example"},
		AllowedMethods: []string{http.MethodPost},
		AllowedHeaders: []string{"authorization", "content-type"},
		MaxAge:         10 * time.Minute,
	})(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/tasks", nil)
	req.Header.Set("origin", "https://dashboard.example")
	req.Header.Set("access-control-request-method", "post")
	req.Header.Set("access-control-request-headers", "Authorization, Content-Type")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	assertHeader(t, rec.Header(), accessControlAllowOriginHeader, "https://dashboard.example")
	assertHeader(t, rec.Header(), accessControlAllowMethodsHeader, http.MethodPost)
	assertHeader(t, rec.Header(), accessControlAllowHeadersHeader, "Authorization, Content-Type")
	assertHeader(t, rec.Header(), accessControlMaxAgeHeader, "600")
	assertHeaderContains(t, rec.Header(), varyHeader, originHeader)
}

func TestCORSStrictPolicyRejectsDisallowedPreflight(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		method string
		header string
	}{
		{
			name:   "origin",
			origin: "https://attacker.example",
			method: http.MethodPost,
			header: "authorization",
		},
		{
			name:   "method",
			origin: "https://dashboard.example",
			method: http.MethodDelete,
			header: "authorization",
		},
		{
			name:   "header",
			origin: "https://dashboard.example",
			method: http.MethodPost,
			header: "x-unexpected",
		},
	}

	handler := CORSMiddleware(CORSConfig{
		AllowedOrigins: []string{"https://dashboard.example"},
		AllowedMethods: []string{http.MethodPost},
		AllowedHeaders: []string{"authorization"},
	})(http.NotFoundHandler())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodOptions, "/tasks", nil)
			req.Header.Set("origin", tt.origin)
			req.Header.Set("access-control-request-method", tt.method)
			req.Header.Set("access-control-request-headers", tt.header)

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("body length = %d, want 0", rec.Body.Len())
			}
			if got := rec.Header().Get(accessControlAllowOriginHeader); got != "" {
				t.Fatalf("allow origin = %q, want empty", got)
			}
		})
	}
}

func TestCORSStrictPolicyOverridesDownstreamCORSHeaders(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{
		AllowedOrigins: []string{"https://dashboard.example"},
		ExposeHeaders:  []string{"x-request-id"},
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(accessControlAllowOriginHeader, wildcardHeaderValue)
		w.Header().Set(accessControlExposeHeadersHeader, "X-Leaked")
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("origin", "https://dashboard.example")

	handler.ServeHTTP(rec, req)

	assertHeader(t, rec.Header(), accessControlAllowOriginHeader, "https://dashboard.example")
	assertHeader(t, rec.Header(), accessControlExposeHeadersHeader, "X-Request-Id")
}

func TestCORSStrictPolicyRemovesDownstreamHeadersForDisallowedOrigin(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{
		AllowedOrigins: []string{"https://dashboard.example"},
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(accessControlAllowOriginHeader, wildcardHeaderValue)
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("origin", "https://attacker.example")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get(accessControlAllowOriginHeader); got != "" {
		t.Fatalf("allow origin = %q, want empty", got)
	}
}

func TestCORSRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config CORSConfig
	}{
		{
			name: "negative max age",
			config: CORSConfig{
				MaxAge: -time.Second,
			},
		},
		{
			name: "invalid status",
			config: CORSConfig{
				OptionsStatus: 99,
			},
		},
		{
			name: "wildcard credentials",
			config: CORSConfig{
				AllowedOrigins:   []string{wildcardHeaderValue},
				AllowCredentials: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			_ = CORSMiddleware(tt.config)
		})
	}
}

func assertHeader(t *testing.T, header http.Header, name, want string) {
	t.Helper()
	if got := header.Get(name); got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}

func assertHeaderContains(t *testing.T, header http.Header, name, want string) {
	t.Helper()
	if slices.Contains(header.Values(name), want) {
		return
	}
	t.Fatalf("%s values = %#v, want %q", name, header.Values(name), want)
}
