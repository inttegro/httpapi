package response

import (
	"net/http"
	"testing"

	callerpkg "github.com/inttegro/httpapi/caller"
	e "github.com/inttegro/httpapi/erreur"
	"github.com/inttegro/httpapi/param"
)

func TestRenderParamErrUsesCustomErrorFields(t *testing.T) {
	target := &testTarget{}
	RenderParamErr(target, &e.ErrInvalidParam{
		Param:   "media.hero_image",
		Mesg:    "wrong purpose",
		Code:    "file_reference_purpose_mismatch",
		Status:  http.StatusServiceUnavailable,
		Cause:   "service_unavailable",
		Type:    e.TypeTransient,
		FixCode: e.FixCodeRepeatSame,
	})

	if target.res.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", target.res.Status, http.StatusServiceUnavailable)
	}

	body, ok := target.res.Body.(ErrRes)
	if !ok {
		t.Fatalf("body = %T, want ErrRes", target.res.Body)
	}
	if body.Err.Code != "file_reference_purpose_mismatch" {
		t.Fatalf("code = %q", body.Err.Code)
	}
	if body.Err.Cause != "service_unavailable" || body.Err.Type != e.TypeTransient || body.Err.FixCode != e.FixCodeRepeatSame {
		t.Fatalf("unexpected error fields: %#v", body.Err)
	}
}

func TestRenderParamErrorUsesStandardErrorFields(t *testing.T) {
	target := &testTarget{}
	RenderParamError(target, param.NewError(
		"customer.email",
		param.CodeMissing,
		"`customer.email` is required",
		nil,
	))

	if target.res.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", target.res.Status, http.StatusBadRequest)
	}

	body, ok := target.res.Body.(ErrRes)
	if !ok {
		t.Fatalf("body = %T, want ErrRes", target.res.Body)
	}
	if body.Err.Code != string(param.CodeMissing) {
		t.Fatalf("code = %q", body.Err.Code)
	}
	if body.Err.Message != "`customer.email` is required" {
		t.Fatalf("message = %q", body.Err.Message)
	}
	if body.Err.Detail != "customer.email" {
		t.Fatalf("detail = %q", body.Err.Detail)
	}
	if body.Err.Cause != e.CauseMissingParam || body.Err.Type != e.TypeInvalidParam {
		t.Fatalf("unexpected error fields: %#v", body.Err)
	}
}

func TestErrorConstructorsReturnStandardResponses(t *testing.T) {
	errRes := Error(
		e.MethodNotAllowed(http.MethodPost, http.MethodGet),
		WithHeader("x-error", "method"),
	)
	if errRes.Status != http.StatusMethodNotAllowed {
		t.Fatalf("error status = %d, want %d", errRes.Status, http.StatusMethodNotAllowed)
	}
	if errRes.ContentType != ApplicationJson {
		t.Fatalf("error content type = %q, want %q", errRes.ContentType, ApplicationJson)
	}
	if errRes.Header.Get("x-error") != "method" {
		t.Fatalf("error header = %q, want method", errRes.Header.Get("x-error"))
	}
	if body := responseErrorBody(t, errRes); body.Code != "method_not_allowed" {
		t.Fatalf("error code = %q, want method_not_allowed", body.Code)
	}

	paramRes := ParamError(param.NewError(
		"customer.email",
		param.CodeMissing,
		"`customer.email` is required",
		nil,
	))
	paramBody := responseErrorBody(t, paramRes)
	if paramRes.Status != http.StatusBadRequest {
		t.Fatalf("param status = %d, want %d", paramRes.Status, http.StatusBadRequest)
	}
	if paramBody.Code != string(param.CodeMissing) || paramBody.Detail != "customer.email" {
		t.Fatalf("param error body = %#v", paramBody)
	}
	if paramBody.Cause != e.CauseMissingParam || paramBody.Type != e.TypeInvalidParam {
		t.Fatalf("param error fields = %#v", paramBody)
	}

	invalidRes := InvalidParamError(&e.ErrInvalidParam{
		Mesg:    "wrong purpose",
		Code:    "file_reference_purpose_mismatch",
		Status:  http.StatusServiceUnavailable,
		Cause:   "service_unavailable",
		Type:    e.TypeTransient,
		FixCode: e.FixCodeRepeatSame,
	})
	invalidBody := responseErrorBody(t, invalidRes)
	if invalidRes.Status != http.StatusServiceUnavailable {
		t.Fatalf("invalid param status = %d, want %d", invalidRes.Status, http.StatusServiceUnavailable)
	}
	if invalidBody.Code != "file_reference_purpose_mismatch" {
		t.Fatalf("invalid param code = %q", invalidBody.Code)
	}
	if invalidBody.Cause != "service_unavailable" || invalidBody.Type != e.TypeTransient || invalidBody.FixCode != e.FixCodeRepeatSame {
		t.Fatalf("invalid param fields = %#v", invalidBody)
	}
}

func TestRenderErrUsesErreurResponseStatus(t *testing.T) {
	target := &testTarget{}
	RenderErr(target, e.MethodNotAllowed(http.MethodPost, http.MethodGet))

	if target.res.Status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", target.res.Status, http.StatusMethodNotAllowed)
	}
	if target.res.ContentType != ApplicationJson {
		t.Fatalf("content_type = %q, want %q", target.res.ContentType, ApplicationJson)
	}

	body, ok := target.res.Body.(ErrRes)
	if !ok {
		t.Fatalf("body = %T, want ErrRes", target.res.Body)
	}
	if body.Err.Code != "method_not_allowed" {
		t.Fatalf("code = %q, want method_not_allowed", body.Err.Code)
	}
	if body.Err.Type != e.TypeInvalidRequest || body.Err.Cause != e.CauseMethodNotAllowed {
		t.Fatalf("unexpected error fields: %#v", body.Err)
	}
}

func TestRenderErrDefaultsNilToUnexpected(t *testing.T) {
	target := &testTarget{}
	RenderErr(target, nil)

	if target.res.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", target.res.Status, http.StatusInternalServerError)
	}

	body, ok := target.res.Body.(ErrRes)
	if !ok {
		t.Fatalf("body = %T, want ErrRes", target.res.Body)
	}
	if body.Err.Code != "request_failed" {
		t.Fatalf("code = %q, want request_failed", body.Err.Code)
	}
}

func TestErrorShapeDescribesAndProjectsStandardErrorBody(t *testing.T) {
	spec := Describe(ErrorShape)
	if spec.Type != TypeObject || len(spec.Attributes) != 1 {
		t.Fatalf("error shape spec = %#v", spec)
	}
	if spec.Attributes[0].Name != "error" || !spec.Attributes[0].Required {
		t.Fatalf("error attribute = %#v", spec.Attributes[0])
	}

	body := ErrorShape.Project(ErrorBody{Err: e.InvalidRequestBody()}, Context{Caller: callerpkg.Caller{}})
	errObject, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("projected error = %#v", body["error"])
	}
	if errObject["code"] != "invalid_request_body" {
		t.Fatalf("code = %#v", errObject["code"])
	}
	if errObject["message"] == nil {
		t.Fatalf("message missing from projected error: %#v", errObject)
	}
}

func responseErrorBody(t *testing.T, res *Res) *e.Error {
	t.Helper()

	body, ok := res.Body.(ErrRes)
	if !ok {
		t.Fatalf("body = %T, want ErrRes", res.Body)
	}
	if body.Err == nil {
		t.Fatal("error body is nil")
	}

	return body.Err
}
