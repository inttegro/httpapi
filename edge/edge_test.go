package edge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testProvider struct {
	name    string
	payload any
}

func (p testProvider) Name() string { return p.name }

func (p testProvider) Extract(*http.Request) (any, bool) {
	return p.payload, p.payload != nil
}

func TestAuditUsesProviderAsDiscriminator(t *testing.T) {
	audit := Extract(
		testProvider{name: "example_edge", payload: map[string]string{"request_id": "edge_123"}},
		httptest.NewRequest(http.MethodGet, "/", nil),
	)
	if audit == nil {
		t.Fatal("Extract returned nil")
	}

	encoded, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if string(got["provider"]) != `"example_edge"` {
		t.Fatalf("provider = %s", got["provider"])
	}
	if _, ok := got["example_edge"]; !ok {
		t.Fatalf("provider payload missing: %s", encoded)
	}
}

func TestExtractRejectsInvalidProviderNames(t *testing.T) {
	for _, name := range []string{"", "Cloudflare", "bad-provider", "provider.name"} {
		if got := Extract(
			testProvider{name: name, payload: struct{}{}},
			httptest.NewRequest(http.MethodGet, "/", nil),
		); got != nil {
			t.Fatalf("Extract(%q) = %#v, want nil", name, got)
		}
	}
}
