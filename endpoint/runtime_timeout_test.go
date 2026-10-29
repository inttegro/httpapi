package endpoint

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	responsepkg "github.com/inttegro/httpapi/response"
)

func TestDefineEndpointTimeoutSpec(t *testing.T) {
	timeout := EndpointTimeoutSpec{
		ReadBody: time.Second,
		Handler:  2 * time.Second,
		Write:    3 * time.Second,
	}

	endpoint := DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/timeouts",
		Handler: noopTranscriptionHandler,
		Timeout: timeout,
	})

	if got := endpoint.TimeoutSpec(); got != timeout {
		t.Fatalf("timeout = %#v, want %#v", got, timeout)
	}
}

func TestDefineEndpointTimeoutDefaults(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method:  POST,
		Handler: noopTranscriptionHandler,
	})

	if got := endpoint.TimeoutSpec(); got != (EndpointTimeoutSpec{}) {
		t.Fatalf("timeout = %#v, want zero value", got)
	}
}

func TestDefineEndpointRejectsNegativeTimeouts(t *testing.T) {
	tests := []struct {
		name    string
		timeout EndpointTimeoutSpec
	}{
		{
			name: "read body",
			timeout: EndpointTimeoutSpec{
				ReadBody: -time.Millisecond,
			},
		},
		{
			name: "handler",
			timeout: EndpointTimeoutSpec{
				Handler: -time.Millisecond,
			},
		},
		{
			name: "write",
			timeout: EndpointTimeoutSpec{
				Write: -time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Fatal("DefineEndpoint did not panic")
				}
			}()

			DefineEndpoint(EndpointSpec{
				Method:  POST,
				Handler: noopTranscriptionHandler,
				Timeout: tt.timeout,
			})
		})
	}
}

func TestEndpointGroupTimeoutDefaultsAndEndpointOverrides(t *testing.T) {
	group := EndpointGroup{
		PathPrefix: "/ops",
		Timeout: EndpointTimeoutSpec{
			ReadBody: time.Second,
			Handler:  2 * time.Second,
			Write:    3 * time.Second,
		},
	}

	group.Add(NewEndpoint(POST, "/sync", noopTranscriptionHandler))
	group.Add(DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/health",
		Handler: noopTranscriptionHandler,
		Timeout: EndpointTimeoutSpec{
			Handler: 4 * time.Second,
		},
	}))

	first := group.Endpoints[0]
	if got, want := first.TimeoutSpec(), group.TimeoutSpec(); got != want {
		t.Fatalf("inherited timeout = %#v, want %#v", got, want)
	}

	second := group.Endpoints[1]
	if got, want := second.TimeoutSpec(), (EndpointTimeoutSpec{
		ReadBody: time.Second,
		Handler:  4 * time.Second,
		Write:    3 * time.Second,
	}); got != want {
		t.Fatalf("overridden timeout = %#v, want %#v", got, want)
	}

	group.ConfigureTimeoutSpec(EndpointTimeoutSpec{
		ReadBody: 5 * time.Second,
		Handler:  6 * time.Second,
		Write:    7 * time.Second,
	})

	if got, want := group.Endpoints[0].TimeoutSpec(), group.TimeoutSpec(); got != want {
		t.Fatalf("changed inherited timeout = %#v, want %#v", got, want)
	}
	if got, want := group.Endpoints[1].TimeoutSpec(), (EndpointTimeoutSpec{
		ReadBody: 5 * time.Second,
		Handler:  4 * time.Second,
		Write:    7 * time.Second,
	}); got != want {
		t.Fatalf("changed overridden timeout = %#v, want %#v", got, want)
	}
}

func TestEndpointHandlerAppliesRuntimeTimeouts(t *testing.T) {
	timeout := EndpointTimeoutSpec{
		ReadBody: 2 * time.Second,
		Handler:  3 * time.Second,
		Write:    4 * time.Second,
	}
	rec := newDeadlineRecorder()
	var sawReadDeadline bool
	var handlerDeadline time.Time
	var handlerDeadlineOK bool

	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/timeouts",
		Handler: func(r *Req) {
			handlerDeadline, handlerDeadlineOK = r.Context().Deadline()
			if string(r.RequestBody()) != `{"ok":true}` {
				t.Fatalf("request body = %q", r.RequestBody())
			}

			r.Res = &Res{
				ContentType: ApplicationJson,
				Status:      http.StatusAccepted,
				SentAt:      time.Now(),
				Header:      http.Header{},
				Body:        map[string]bool{"ok": true},
			}
		},
		Timeout: timeout,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(POST, "/timeouts", nil).WithContext(ctx)
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	req.Body = &deadlineAwareBody{
		reader:            strings.NewReader(`{"ok":true}`),
		rec:               rec,
		sawActiveDeadline: &sawReadDeadline,
	}

	before := time.Now()
	endpoint.Handler()(rec, req)
	after := time.Now()

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !handlerDeadlineOK {
		t.Fatal("handler context did not have a deadline")
	}
	assertDeadlineBetween(t, "handler", handlerDeadline, before.Add(timeout.Handler), after.Add(timeout.Handler))
	if !sawReadDeadline {
		t.Fatal("request body was read before an active read deadline was set")
	}
	if len(rec.readDeadlines) != 2 {
		t.Fatalf("read deadlines = %d, want set and clear", len(rec.readDeadlines))
	}
	assertDeadlineBetween(t, "read body", rec.readDeadlines[0], before.Add(timeout.ReadBody), after.Add(timeout.ReadBody))
	if !rec.readDeadlines[1].IsZero() {
		t.Fatalf("read deadline was not cleared: %v", rec.readDeadlines[1])
	}
	if len(rec.writeDeadlines) != 2 {
		t.Fatalf("write deadlines = %d, want set and clear", len(rec.writeDeadlines))
	}
	assertDeadlineBetween(t, "write", rec.writeDeadlines[0], before.Add(timeout.Write), after.Add(timeout.Write))
	if !rec.writeDeadlines[1].IsZero() {
		t.Fatalf("write deadline was not cleared: %v", rec.writeDeadlines[1])
	}
}

func TestEndpointHandlerRejectsMethodBeforeReadingBody(t *testing.T) {
	rec := newDeadlineRecorder()
	var readCalled bool
	endpoint := DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/timeouts",
		Handler: noopTranscriptionHandler,
		Timeout: EndpointTimeoutSpec{
			ReadBody: time.Second,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(GET, "/timeouts", nil).WithContext(ctx)
	req.Body = &deadlineAwareBody{
		reader:     strings.NewReader(`{"ok":true}`),
		readCalled: &readCalled,
	}

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
	if readCalled {
		t.Fatal("request body was read before method rejection")
	}
	if len(rec.readDeadlines) != 0 {
		t.Fatalf("read deadlines = %d, want none before method rejection", len(rec.readDeadlines))
	}
}

func TestEndpointHandlerRendersTimeoutWhenContextBudgetExpires(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/timeouts",
		Handler: func(r *Req) {
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
				t.Fatal("handler context deadline was not reached")
			}
		},
		Timeout: EndpointTimeoutSpec{
			Handler: time.Nanosecond,
		},
	})

	rec := newDeadlineRecorder()
	req := httptest.NewRequest(POST, "/timeouts", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"request_timeout"`) {
		t.Fatalf("body = %s, want request_timeout code", rec.Body.String())
	}
}

func TestEndpointHandlerUsesCustomTimeoutHandler(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/timeouts",
		Handler: func(r *Req) {
			<-r.Context().Done()
		},
		Timeout: EndpointTimeoutSpec{
			Handler: time.Nanosecond,
		},
		TimeoutHandler: func(r *Req) {
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]string{
				"code": "queued_after_timeout",
			})
		},
	})

	rec := newDeadlineRecorder()
	req := httptest.NewRequest(POST, "/timeouts", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"queued_after_timeout"`) {
		t.Fatalf("body = %s, want custom timeout body", rec.Body.String())
	}
}

func TestEndpointHandlerFallsBackWhenCustomTimeoutHandlerDoesNotRender(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/timeouts",
		Handler: func(r *Req) {
			<-r.Context().Done()
		},
		Timeout: EndpointTimeoutSpec{
			Handler: time.Nanosecond,
		},
		TimeoutHandler: func(*Req) {},
	})

	rec := newDeadlineRecorder()
	req := httptest.NewRequest(POST, "/timeouts", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	endpoint.Handler()(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"request_timeout"`) {
		t.Fatalf("body = %s, want request_timeout code", rec.Body.String())
	}
}

func TestEndpointGroupTimeoutHandlerAppliesToResolvedHandler(t *testing.T) {
	group := EndpointGroup{
		PathPrefix: "/ops",
		Timeout: EndpointTimeoutSpec{
			Handler: time.Nanosecond,
		},
		TimeoutHandler: func(r *Req) {
			responsepkg.RenderJSON(r, http.StatusAccepted, map[string]string{
				"code": "group_timeout",
			})
		},
	}
	group.Add(NewEndpoint(POST, "/sync", func(r *Req) {
		<-r.Context().Done()
	}))

	rec := newDeadlineRecorder()
	req := httptest.NewRequest(POST, "/ops/sync", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	group.Endpoints[0].Handler()(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"group_timeout"`) {
		t.Fatalf("body = %s, want group timeout body", rec.Body.String())
	}
}

func TestEndpointGroupTimeoutHandlerCanBeCleared(t *testing.T) {
	group := EndpointGroup{
		PathPrefix: "/ops",
		Timeout: EndpointTimeoutSpec{
			Handler: time.Nanosecond,
		},
	}
	group.ConfigureTimeoutHandler(func(r *Req) {
		responsepkg.RenderJSON(r, http.StatusAccepted, map[string]string{
			"code": "group_timeout",
		})
	})
	group.Add(NewEndpoint(POST, "/sync", func(r *Req) {
		<-r.Context().Done()
	}))
	group.ConfigureTimeoutHandler(nil)

	rec := newDeadlineRecorder()
	req := httptest.NewRequest(POST, "/ops/sync", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)

	group.Endpoints[0].Handler()(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"request_timeout"`) {
		t.Fatalf("body = %s, want request_timeout code", rec.Body.String())
	}
}

type deadlineRecorder struct {
	HeaderMap      http.Header
	Body           bytes.Buffer
	Code           int
	readDeadlines  []time.Time
	writeDeadlines []time.Time
}

func newDeadlineRecorder() *deadlineRecorder {
	return &deadlineRecorder{HeaderMap: http.Header{}}
}

func (r *deadlineRecorder) Header() http.Header {
	return r.HeaderMap
}

func (r *deadlineRecorder) WriteHeader(status int) {
	r.Code = status
}

func (r *deadlineRecorder) Write(body []byte) (int, error) {
	if r.Code == 0 {
		r.Code = http.StatusOK
	}
	return r.Body.Write(body)
}

func (r *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	r.readDeadlines = append(r.readDeadlines, deadline)
	return nil
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.writeDeadlines = append(r.writeDeadlines, deadline)
	return nil
}

type deadlineAwareBody struct {
	reader            *strings.Reader
	rec               *deadlineRecorder
	sawActiveDeadline *bool
	readCalled        *bool
}

func (b *deadlineAwareBody) Read(p []byte) (int, error) {
	if b.readCalled != nil {
		*b.readCalled = true
	}
	if b.sawActiveDeadline != nil && b.rec != nil && activeReadDeadline(b.rec) {
		*b.sawActiveDeadline = true
	}

	return b.reader.Read(p)
}

func (b *deadlineAwareBody) Close() error {
	return nil
}

func activeReadDeadline(rec *deadlineRecorder) bool {
	if len(rec.readDeadlines) == 0 {
		return false
	}

	return !rec.readDeadlines[len(rec.readDeadlines)-1].IsZero()
}

func assertDeadlineBetween(
	t *testing.T,
	name string,
	got time.Time,
	earliest time.Time,
	latest time.Time,
) {
	t.Helper()

	if got.Before(earliest) || got.After(latest) {
		t.Fatalf(
			"%s deadline = %v, want between %v and %v",
			name,
			got,
			earliest,
			latest,
		)
	}
}
