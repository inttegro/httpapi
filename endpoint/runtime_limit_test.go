package endpoint

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	responsepkg "github.com/inttegro/httpapi/response"
)

func TestDefineEndpointLimitsSpec(t *testing.T) {
	limits := EndpointLimitsSpec{MaxRequestBytes: 4096}

	endpoint := DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/limits",
		Handler: noopTranscriptionHandler,
		Limits:  limits,
	})

	if got := endpoint.LimitsSpec(); got != limits {
		t.Fatalf("limits = %#v, want %#v", got, limits)
	}
}

func TestDefineEndpointRejectsNegativeLimits(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("DefineEndpoint did not panic")
		}
	}()

	DefineEndpoint(EndpointSpec{
		Method:  POST,
		Handler: noopTranscriptionHandler,
		Limits: EndpointLimitsSpec{
			MaxRequestBytes: -1,
		},
	})
}

func TestEndpointGroupLimitDefaultsAndEndpointOverrides(t *testing.T) {
	group := EndpointGroup{
		PathPrefix: "/ops",
		Limits: EndpointLimitsSpec{
			MaxRequestBytes: 1024,
		},
	}

	group.Add(NewEndpoint(POST, "/sync", noopTranscriptionHandler))
	group.Add(DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/bulk",
		Handler: noopTranscriptionHandler,
		Limits: EndpointLimitsSpec{
			MaxRequestBytes: 4096,
		},
	}))

	if got, want := group.Endpoints[0].LimitsSpec(), group.LimitsSpec(); got != want {
		t.Fatalf("inherited limits = %#v, want %#v", got, want)
	}
	if got, want := group.Endpoints[1].LimitsSpec().MaxRequestBytes, int64(4096); got != want {
		t.Fatalf("overridden max request bytes = %d, want %d", got, want)
	}

	group.ConfigureLimitsSpec(EndpointLimitsSpec{MaxRequestBytes: 2048})

	if got, want := group.Endpoints[0].LimitsSpec(), group.LimitsSpec(); got != want {
		t.Fatalf("changed inherited limits = %#v, want %#v", got, want)
	}
	if got, want := group.Endpoints[1].LimitsSpec().MaxRequestBytes, int64(4096); got != want {
		t.Fatalf("changed overridden max request bytes = %d, want %d", got, want)
	}
}

func TestEndpointHandlerRejectsHeadersOverRequestLimit(t *testing.T) {
	var handlerCalled bool
	req := httptest.NewRequest(POST, "/limits", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	req.Header.Set("X-Large", strings.Repeat("x", 128))
	limit := endpointRequestEnvelopeBytes(req) - 1
	endpoint := limitedEndpoint(limit, &handlerCalled)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if handlerCalled {
		t.Fatal("handler ran after request limit rejection")
	}
	if !strings.Contains(rec.Body.String(), `"code":"request_too_large"`) {
		t.Fatalf("body = %s, want request_too_large code", rec.Body.String())
	}
}

func TestEndpointHandlerRejectsKnownBodyOverRequestLimitBeforeReading(t *testing.T) {
	var handlerCalled bool
	var readCalled bool
	body := &trackingReadCloser{
		reader:     strings.NewReader(strings.Repeat("a", 10)),
		readCalled: &readCalled,
	}
	req := httptest.NewRequest(POST, "/limits", body)
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	req.ContentLength = 10
	endpoint := limitedEndpoint(endpointRequestEnvelopeBytes(req)+5, &handlerCalled)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if handlerCalled {
		t.Fatal("handler ran after request limit rejection")
	}
	if readCalled {
		t.Fatal("body was read before known oversized body rejection")
	}
}

func TestEndpointHandlerRejectsChunkedBodyOverRequestLimit(t *testing.T) {
	var handlerCalled bool
	req := httptest.NewRequest(POST, "/limits", strings.NewReader(strings.Repeat("a", 10)))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	req.ContentLength = -1
	endpoint := limitedEndpoint(endpointRequestEnvelopeBytes(req)+5, &handlerCalled)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if handlerCalled {
		t.Fatal("handler ran after request limit rejection")
	}
	if !strings.Contains(rec.Body.String(), `"code":"request_too_large"`) {
		t.Fatalf("body = %s, want request_too_large code", rec.Body.String())
	}
}

func TestEndpointHandlerAcceptsRequestAtRequestLimit(t *testing.T) {
	body := `{"ok":true}`
	var handlerCalled bool
	req := httptest.NewRequest(POST, "/limits?trace=1", strings.NewReader(body))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	endpoint := limitedEndpoint(endpointRequestEnvelopeBytes(req)+int64(len(body)), &handlerCalled)

	rec := httptest.NewRecorder()
	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !handlerCalled {
		t.Fatal("handler did not run")
	}
}

func limitedEndpoint(limit int64, called *bool) Endpoint {
	return DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/limits",
		Handler: func(r *Req) {
			if called != nil {
				*called = true
			}
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]bool{"ok": true})
		},
		Limits: EndpointLimitsSpec{
			MaxRequestBytes: limit,
		},
	})
}

type trackingReadCloser struct {
	reader     *strings.Reader
	readCalled *bool
	readErr    error
}

func (b *trackingReadCloser) Read(p []byte) (int, error) {
	if b.readCalled != nil {
		*b.readCalled = true
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

func (b *trackingReadCloser) Close() error { return nil }

var _ io.ReadCloser = (*trackingReadCloser)(nil)
