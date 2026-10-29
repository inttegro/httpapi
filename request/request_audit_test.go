package request

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	callerpkg "github.com/inttegro/httpapi/caller"
	responsepkg "github.com/inttegro/httpapi/response"
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

func TestReqMarshalJSONWritesAuthFailuresOnlyUnderAuth(t *testing.T) {
	authenticationFailure := &AuthFailure{
		Type:    authAuditTypeBearerKey,
		Code:    "bearer_session_failed",
		Message: "Bearer token could not be exchanged for a session",
	}
	authorizationFailure := &AuthFailure{
		Type:    authAuditTypeEndpoint,
		Code:    "capability_denied",
		Message: "endpoint access denied",
	}
	req := Req{
		AuthFailure:          authenticationFailure,
		AuthorizationFailure: authorizationFailure,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request audit: %v", err)
	}

	var audit map[string]json.RawMessage
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("decode request audit: %v", err)
	}
	for _, name := range []string{"auth_failure", "authorization_failure"} {
		if _, ok := audit[name]; ok {
			t.Fatalf("request audit included root %q: %s", name, data)
		}
	}

	var auth AuthAudit
	if err := json.Unmarshal(audit["auth"], &auth); err != nil {
		t.Fatalf("decode auth audit: %v", err)
	}
	if auth.Failure == nil || auth.Failure.Code != authenticationFailure.Code {
		t.Fatalf("auth.failure = %#v, want code %q", auth.Failure, authenticationFailure.Code)
	}
	if auth.AuthenticationFailure == nil || auth.AuthenticationFailure.Code != authenticationFailure.Code {
		t.Fatalf("auth.authentication_failure = %#v, want code %q", auth.AuthenticationFailure, authenticationFailure.Code)
	}
	if auth.AuthorizationFailure == nil || auth.AuthorizationFailure.Code != authorizationFailure.Code {
		t.Fatalf("auth.authorization_failure = %#v, want code %q", auth.AuthorizationFailure, authorizationFailure.Code)
	}
}

func TestReqMarshalJSONRecordsCredentialMetadataWithoutMaterial(t *testing.T) {
	directToken := strings.Repeat("d", 64)
	forwardedToken := strings.Repeat("f", 64)
	httpReq := httptest.NewRequest(http.MethodPost, "/orders/lookup", strings.NewReader("{}"))
	httpReq.Header.Set(authHeaderKey, "Bearer "+directToken)
	httpReq.Header.Set(fwdAuthHeaderKey, "Bearer "+forwardedToken)
	req := Req{Req: httpReq}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request audit: %v", err)
	}

	var audit struct {
		Auth AuthAudit `json:"auth"`
	}
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("decode request audit: %v", err)
	}
	if audit.Auth.Credential == nil {
		t.Fatal("auth audit omitted presented credential metadata")
	}
	if !audit.Auth.Credential.Presented || audit.Auth.Credential.Scheme != "bearer" {
		t.Fatalf("credential audit = %#v, want presented bearer", audit.Auth.Credential)
	}
	if audit.Auth.Credential.Source != fwdAuthHeaderKey {
		t.Fatalf("credential source = %q, want %q", audit.Auth.Credential.Source, fwdAuthHeaderKey)
	}
	wantHash := sha256.Sum256([]byte(forwardedToken))
	wantFingerprint := "sha256:" + hex.EncodeToString(wantHash[:])
	if audit.Auth.Credential.Fingerprint != wantFingerprint {
		t.Fatalf("credential fingerprint = %q, want %q", audit.Auth.Credential.Fingerprint, wantFingerprint)
	}
	if strings.Contains(string(data), directToken) || strings.Contains(string(data), forwardedToken) {
		t.Fatalf("request audit leaked credential material: %s", data)
	}
}

func TestReqAuthAuditDoesNotFingerprintShortCredentialMaterial(t *testing.T) {
	httpReq := httptest.NewRequest(http.MethodPost, "/orders/lookup", strings.NewReader("{}"))
	httpReq.Header.Set(authHeaderKey, "Bearer short-token")

	audit := (&Req{Req: httpReq}).AuthAudit()
	if audit.Credential == nil || !audit.Credential.Presented {
		t.Fatal("auth audit omitted short presented credential metadata")
	}
	if audit.Credential.Fingerprint != "" {
		t.Fatalf("short credential fingerprint = %q, want empty", audit.Credential.Fingerprint)
	}
}
