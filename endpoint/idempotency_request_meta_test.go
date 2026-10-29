package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	e "github.com/inttegro/httpapi/erreur"
	"github.com/inttegro/httpapi/param"
	"github.com/inttegro/httpapi/response"
)

type noopIdempotencyStore struct{}

func (noopIdempotencyStore) Reserve(
	context.Context,
	*IdempotencyRecord,
) (*IdempotencyRecord, error) {
	return nil, nil
}

func (noopIdempotencyStore) Complete(context.Context, *IdempotencyRecord) error {
	return nil
}

func (noopIdempotencyStore) Release(context.Context, string, string) error {
	return nil
}

func TestIdempotentEndpointStripsRequestMetaBeforeHandlerParsing(t *testing.T) {
	restoreStore := setIdempotencyStoreForTest(noopIdempotencyStore{})
	defer restoreStore()

	request := param.JSON[string]().
		Param(param.Required("message", param.String()).Null(param.NullRejected).Parse(param.NonEmptyTrimmedString)).
		Parse(func(values param.Values) (string, error) {
			return param.Must[string](values, "message"), nil
		})

	var gotMessage string
	var gotIdemKey string
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/messages/send",
		Handler: func(r *Req) {
			params, err := request.Parse(r)
			if err != nil {
				response.RenderParamError(r, err)
				return
			}

			gotMessage = params
			gotIdemKey = r.IdemKey
			response.RenderJSON(r, http.StatusOK, map[string]string{"message": params})
		},
		Idempotency: EndpointIdempotencySpec{
			Enabled: true,
			ScopeResolver: func(*Req) (string, *e.ErrInvalidParam) {
				return "app_123", nil
			},
		},
		Request: RequestBody(request),
	})

	req := httptest.NewRequest(
		POST,
		"/messages/send",
		strings.NewReader(`{"request_meta":{"idempotency_key":"body-key","trace":"abc"},"message":"hello"}`),
	)
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", res.Code, http.StatusOK, res.Body.String())
	}
	if gotMessage != "hello" {
		t.Fatalf("message = %q, want hello", gotMessage)
	}
	if gotIdemKey != "body-key" {
		t.Fatalf("idempotency key = %q, want body-key", gotIdemKey)
	}
}
