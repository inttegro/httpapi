package endpoint

import (
	"net/http"
	"testing"

	"github.com/inttegro/httpapi/param"
	"github.com/inttegro/httpapi/response"
)

func TestHandlerFromResponderRendersReturnedResponse(t *testing.T) {
	handler := HandlerFromResponder(func(*Req) *response.Res {
		return response.JSON(http.StatusAccepted, map[string]string{
			"status": "accepted",
		})
	})

	req := &Req{}
	handler(req)

	if req.Res == nil || req.Res.Status != http.StatusAccepted {
		t.Fatalf("response = %#v", req.Res)
	}
	body, ok := req.Res.Body.(map[string]string)
	if !ok || body["status"] != "accepted" {
		t.Fatalf("body = %#v", req.Res.Body)
	}
}

func TestHandlerFromResponderDefaultsNilUnrenderedResponse(t *testing.T) {
	handler := HandlerFromResponder(func(*Req) *response.Res {
		return nil
	})

	req := &Req{}
	handler(req)

	if req.Res == nil || req.Res.Status != http.StatusInternalServerError {
		t.Fatalf("response = %#v", req.Res)
	}
	body, ok := req.Res.Body.(response.ErrRes)
	if !ok {
		t.Fatalf("body = %T, want response.ErrRes", req.Res.Body)
	}
	if body.Err == nil || body.Err.Code != "request_failed" {
		t.Fatalf("error body = %#v", body.Err)
	}
}

func TestHandlerFromResponderPreservesAlreadyRenderedNilResponse(t *testing.T) {
	handler := HandlerFromResponder(func(r *Req) *response.Res {
		response.RenderNoContent(r)
		return nil
	})

	req := &Req{}
	handler(req)

	if req.Res == nil || req.Res.Status != http.StatusNoContent {
		t.Fatalf("response = %#v", req.Res)
	}
}

func TestHandlerWithRequestResponderParsesBeforeInvokingResponder(t *testing.T) {
	var got typedJSONParams
	handler := HandlerWithRequestResponder(
		typedJSONRequest(),
		func(_ *Req, params typedJSONParams) *response.Res {
			got = params
			return response.NoContent()
		},
	)

	req := &Req{Body: []byte(`{"name":" Ada "}`)}
	handler(req)

	if got.Name != "Ada" {
		t.Fatalf("name = %q, want Ada", got.Name)
	}
	if req.Res == nil || req.Res.Status != http.StatusNoContent {
		t.Fatalf("response = %#v", req.Res)
	}
}

func TestHandlerWithRequestResponderReturnsParamErrors(t *testing.T) {
	called := false
	handler := HandlerWithRequestResponder(
		typedJSONRequest(),
		func(*Req, typedJSONParams) *response.Res {
			called = true
			return response.NoContent()
		},
	)

	req := &Req{Body: []byte(`{}`)}
	handler(req)

	if called {
		t.Fatal("typed responder was called")
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

func TestDefineEndpointUsesResponder(t *testing.T) {
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/respond",
		Respond: func(*Req) *response.Res {
			return response.NoContent()
		},
	})

	req := &Req{}
	endpoint.rawHandler(req)

	if req.Res == nil || req.Res.Status != http.StatusNoContent {
		t.Fatalf("response = %#v", req.Res)
	}
}

func TestDefineEndpointRejectsHandlerAndResponder(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("DefineEndpoint did not panic")
		}
	}()

	DefineEndpoint(EndpointSpec{
		Method:  POST,
		Handler: noopTranscriptionHandler,
		Respond: func(*Req) *response.Res {
			return response.NoContent()
		},
	})
}

func TestDefineJSONEndpointUsesResponder(t *testing.T) {
	endpoint := DefineJSONEndpoint(JSONEndpointSpec[typedJSONParams]{
		Method:  POST,
		Path:    "/typed",
		Request: typedJSONRequest(),
		Respond: func(_ *Req, params typedJSONParams) *response.Res {
			return response.JSON(http.StatusCreated, params)
		},
	})

	req := &Req{Body: []byte(`{"name":"Ada"}`)}
	endpoint.rawHandler(req)

	if req.Res == nil || req.Res.Status != http.StatusCreated {
		t.Fatalf("response = %#v", req.Res)
	}
	body, ok := req.Res.Body.(typedJSONParams)
	if !ok || body.Name != "Ada" {
		t.Fatalf("body = %#v", req.Res.Body)
	}
}

func TestDefineJSONEndpointRejectsHandlerAndResponder(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("DefineJSONEndpoint did not panic")
		}
	}()

	DefineJSONEndpoint(JSONEndpointSpec[typedJSONParams]{
		Method:  POST,
		Request: typedJSONRequest(),
		Handler: func(*Req, typedJSONParams) {},
		Respond: func(*Req, typedJSONParams) *response.Res {
			return response.NoContent()
		},
	})
}
