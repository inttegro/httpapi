package endpoint

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	responsepkg "github.com/inttegro/httpapi/response"
)

func TestEndpointWritesStreamResponse(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/stream",
		Handler: func(r *Req) {
			responsepkg.RenderStream(
				r,
				http.StatusAccepted,
				"text/plain",
				http.Header{"x-stream": {"yes"}},
				io.NopCloser(strings.NewReader("streamed")),
			)
		},
	})

	req := httptest.NewRequest(POST, "/stream", strings.NewReader("{}"))
	req.Header.Set("content-type", ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusAccepted, res.Body.String())
	}
	if res.Body.String() != "streamed" {
		t.Fatalf("body = %q, want streamed", res.Body.String())
	}
	if res.Header().Get("content-type") != "text/plain" {
		t.Fatalf("content type = %q, want text/plain", res.Header().Get("content-type"))
	}
	if res.Header().Get("x-stream") != "yes" {
		t.Fatalf("x-stream header = %q, want yes", res.Header().Get("x-stream"))
	}
}
