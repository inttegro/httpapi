package request

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	callerpkg "github.com/inttegro/httpapi/caller"
	responsepkg "github.com/inttegro/httpapi/response"
)

type callerProjectionBody struct{}

func (callerProjectionBody) Project(context responsepkg.Context) any {
	return map[string]string{"caller": context.Caller.Name()}
}

func TestNewReqAttachesCallerFromContext(t *testing.T) {
	worker := callerpkg.Define("worker")
	req := httptest.NewRequest("POST", "/records/new", strings.NewReader(`{}`)).
		WithContext(ContextWithCaller(context.Background(), worker))

	parsed := NewReq(req)
	if parsed == nil {
		t.Fatal("NewReq returned nil")
	}
	if parsed.Caller != worker {
		t.Fatalf("caller = %q, want %q", parsed.Caller, worker)
	}
	if parsed.RequestCaller() != worker {
		t.Fatalf("request caller = %q, want %q", parsed.RequestCaller(), worker)
	}
}

func TestResponseCallerDefaultsToRequestCaller(t *testing.T) {
	worker := callerpkg.Define("worker")
	parsed := &Req{}
	parsed.AttachCaller(worker)

	if parsed.ResponseCaller() != worker {
		t.Fatalf("response caller = %q, want %q", parsed.ResponseCaller(), worker)
	}
}

func TestNewReqKeepsRequestAndResponseCallersSeparate(t *testing.T) {
	worker := callerpkg.Define("worker")
	publicAPI := callerpkg.Define("public-api")
	policy := callerpkg.DefineProjectionPolicy(callerpkg.Descends(worker, publicAPI))
	ctx := ContextWithCaller(context.Background(), worker)
	ctx, err := ContextWithResponseProjection(ctx, policy, publicAPI)
	if err != nil {
		t.Fatalf("ContextWithResponseProjection error = %v", err)
	}
	req := httptest.NewRequest("POST", "/records/new", strings.NewReader(`{}`)).WithContext(ctx)

	parsed := NewReq(req)
	if parsed == nil {
		t.Fatal("NewReq returned nil")
	}
	if parsed.RequestCaller() != worker {
		t.Fatalf("request caller = %q, want %q", parsed.RequestCaller(), worker)
	}
	if parsed.ResponseCaller() != publicAPI {
		t.Fatalf("response caller = %q, want %q", parsed.ResponseCaller(), publicAPI)
	}
}

func TestResponseBodyUsesResponseCaller(t *testing.T) {
	worker := callerpkg.Define("worker")
	publicAPI := callerpkg.Define("public-api")
	policy := callerpkg.DefineProjectionPolicy(callerpkg.Descends(worker, publicAPI))
	parsed := &Req{}
	parsed.AttachCaller(worker)
	if err := parsed.ProjectResponseAs(policy, publicAPI); err != nil {
		t.Fatalf("ProjectResponseAs error = %v", err)
	}
	parsed.SetResponse(responsepkg.JSON(http.StatusOK, callerProjectionBody{}))

	body, err := parsed.ResponseBody()
	if err != nil {
		t.Fatalf("ResponseBody error = %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if got["caller"] != publicAPI.Name() {
		t.Fatalf("encoded caller = %q, want %q", got["caller"], publicAPI)
	}
}

func TestResponseProjectionCannotElevateRequestCaller(t *testing.T) {
	worker := callerpkg.Define("worker")
	publicAPI := callerpkg.Define("public-api")
	policy := callerpkg.DefineProjectionPolicy(callerpkg.Descends(worker, publicAPI))
	parsed := &Req{}
	parsed.AttachCaller(publicAPI)

	if err := parsed.ProjectResponseAs(policy, worker); !errors.Is(err, callerpkg.ErrProjectionElevation) {
		t.Fatalf("ProjectResponseAs error = %v, want ErrProjectionElevation", err)
	}
	if parsed.ResponseCaller() != publicAPI {
		t.Fatalf("response caller = %q, want unchanged %q", parsed.ResponseCaller(), publicAPI)
	}
}

func TestContextResponseProjectionDoesNotMutateOnError(t *testing.T) {
	worker := callerpkg.Define("worker")
	publicAPI := callerpkg.Define("public-api")
	policy := callerpkg.DefineProjectionPolicy(callerpkg.Descends(worker, publicAPI))
	ctx := ContextWithCaller(context.Background(), publicAPI)

	projected, err := ContextWithResponseProjection(ctx, policy, worker)
	if !errors.Is(err, callerpkg.ErrProjectionElevation) {
		t.Fatalf("ContextWithResponseProjection error = %v, want ErrProjectionElevation", err)
	}
	if projected != ctx {
		t.Fatal("ContextWithResponseProjection mutated context after failed resolution")
	}
	if got := responseCallerFromContext(projected); got.Defined() {
		t.Fatalf("response caller = %q, want undefined", got)
	}
}
