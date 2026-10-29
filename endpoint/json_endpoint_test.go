package endpoint

import (
	"net/http"
	"testing"

	"github.com/inttegro/httpapi/param"
	"github.com/inttegro/httpapi/response"
)

type typedJSONParams struct {
	Name string
}

func typedJSONRequest() *param.Request[typedJSONParams] {
	return param.JSON[typedJSONParams]().
		Param(param.Required("name", param.String()).Parse(param.TrimmedString)).
		Parse(func(values param.Values) (typedJSONParams, error) {
			return typedJSONParams{Name: param.Must[string](values, "name")}, nil
		})
}

func TestHandlerWithRequestParsesBeforeInvokingHandler(t *testing.T) {
	var got typedJSONParams
	handler := HandlerWithRequest(typedJSONRequest(), func(r *Req, params typedJSONParams) {
		got = params
		response.Render(r, response.Empty(http.StatusNoContent))
	})

	req := &Req{Body: []byte(`{"name":" Ada "}`)}
	handler(req)

	if got.Name != "Ada" {
		t.Fatalf("name = %q, want Ada", got.Name)
	}
	if req.Res == nil || req.Res.Status != http.StatusNoContent {
		t.Fatalf("response = %#v", req.Res)
	}
}

func TestHandlerWithRequestRendersParamErrors(t *testing.T) {
	called := false
	handler := HandlerWithRequest(typedJSONRequest(), func(*Req, typedJSONParams) {
		called = true
	})

	req := &Req{Body: []byte(`{}`)}
	handler(req)

	if called {
		t.Fatal("typed handler was called")
	}
	if req.Res == nil || req.Res.Status != http.StatusBadRequest {
		t.Fatalf("response = %#v", req.Res)
	}
	body, ok := req.Res.Body.(response.ErrRes)
	if !ok {
		t.Fatalf("body = %T, want response.ErrRes", req.Res.Body)
	}
	if body.Err.Code != string(param.CodeMissing) || body.Err.Detail != "name" {
		t.Fatalf("error body = %#v", body.Err)
	}
}

func TestDefineJSONEndpointBindsRuntimeHandlerAndRequestContract(t *testing.T) {
	endpoint := DefineJSONEndpoint(JSONEndpointSpec[typedJSONParams]{
		Method:  POST,
		Path:    "/typed",
		Request: typedJSONRequest(),
		Handler: func(r *Req, params typedJSONParams) {
			response.Render(r, response.JSON(http.StatusOK, params))
		},
		Operation: OperationSpec{
			ID:      "typedEndpoint",
			Summary: "Typed endpoint",
		},
	})

	if endpoint.Method() != POST || endpoint.Pattern() != "/typed" {
		t.Fatalf("endpoint route = %s %s", endpoint.Method(), endpoint.Pattern())
	}
	if endpoint.RequestContract().Body.Type != param.TypeObject {
		t.Fatalf("request contract = %#v", endpoint.RequestContract())
	}

	req := &Req{Body: []byte(`{"name":"Ada"}`)}
	endpoint.rawHandler(req)
	if req.Res == nil || req.Res.Status != http.StatusOK {
		t.Fatalf("response = %#v", req.Res)
	}
}
