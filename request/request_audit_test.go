package request

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	callerpkg "github.com/zebodotdev/httpapi/caller"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

func TestNewReqRestoresRequestBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("multipart-body"))

	wrapped := NewReq(req)
	if wrapped == nil {
		t.Fatal("request was not wrapped")
	}
	if string(wrapped.Body) != "multipart-body" {
		t.Fatalf("buffered body = %q, want multipart-body", wrapped.Body)
	}

	restored, err := io.ReadAll(wrapped.Req.Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if string(restored) != "multipart-body" {
		t.Fatalf("restored body = %q, want multipart-body", restored)
	}
}

func TestReqMarshalJSONRedactsAuditSensitiveData(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/files/create?token=query-secret#fragment-secret",
		strings.NewReader(`{"secret":"body-secret"}`),
	)
	req.Header.Set(authHeaderKey, "Bearer auth-secret")
	req.Header.Set(idempotencyHeaderKey, "idem-secret")
	req.Header.Set("Cookie", "cookie-secret")
	req.Header.Set("Referer", "https://commerce.example.test/page?token=referer-secret#referer-fragment-secret")

	wrapped := NewReq(req)
	if wrapped == nil {
		t.Fatal("request was not wrapped")
	}
	wrapped.Res = &responsepkg.Res{
		ContentType: responsepkg.ApplicationJson,
		Status:      http.StatusCreated,
		Header: http.Header{
			"Location":   []string{"https://signed.example.test/private-object?token=response-location-secret"},
			"Set-Cookie": []string{"session=response-cookie-secret"},
		},
		Body: map[string]string{"secret": "response-body-secret"},
	}

	data, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("marshal request audit: %v", err)
	}
	rendered := string(data)
	for _, forbidden := range []string{
		"auth-secret",
		"body-secret",
		"cookie-secret",
		"idem-secret",
		"query-secret",
		"fragment-secret",
		"referer-secret",
		"referer-fragment-secret",
		"response-location-secret",
		"response-cookie-secret",
		"response-body-secret",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("request audit leaked %q in %s", forbidden, rendered)
		}
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode request audit: %v", err)
	}
	response, ok := raw["response"].(map[string]any)
	if !ok {
		t.Fatalf("response audit missing or wrong type: %s", data)
	}
	if _, ok := response["body"]; ok {
		t.Fatalf("response audit included raw body: %s", data)
	}
	if response["body_present"] != true {
		t.Fatalf("response body presence missing: %s", data)
	}
}

func TestReqMarshalJSONIncludesExplicitResponseCaller(t *testing.T) {
	wrapped := NewReq(httptest.NewRequest(http.MethodGet, "/records", nil))
	worker := callerpkg.Define("worker")
	publicAPI := callerpkg.Define("public-api")
	wrapped.AttachCaller(worker)
	policy := callerpkg.DefineProjectionPolicy(callerpkg.Descends(worker, publicAPI))
	if err := wrapped.ProjectResponseAs(policy, publicAPI); err != nil {
		t.Fatalf("ProjectResponseAs error = %v", err)
	}

	data, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("marshal request audit: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode request audit: %v", err)
	}
	if raw["caller"] != "worker" {
		t.Fatalf("caller = %#v, want worker", raw["caller"])
	}
	if raw["response_caller"] != "public-api" {
		t.Fatalf("response_caller = %#v, want public-api", raw["response_caller"])
	}
}
