package cloudflare

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/inttegro/httpapi/edge"
)

func TestProviderExtractsBoundedCloudflareEvidence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://api.inttegro.com/orders", nil)
	req.Header.Set("CF-Ray", "94E8AC88A48E1C5D-DUB")
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	req.Header.Set("CF-Connecting-IPv6", "2001:db8::1")
	req.Header.Set("CF-Pseudo-IPv4", "192.0.2.4")
	req.Header.Set("CF-IPCountry", "ie")
	req.Header.Set("CF-Visitor", `{"scheme":"https","secret":"not-projected"}`)
	req.Header.Set("CF-Worker", "storefront.example.workers.dev")
	req.Header.Set("CDN-Loop", "cloudflare; loops=1")
	req.Header.Set("CF-EW-Via", "15")

	audit := edge.Extract(Provider{}, req)
	if audit == nil || audit.Provider() != providerName {
		t.Fatalf("audit = %#v", audit)
	}
	payload, ok := Provider{}.Extract(req)
	if !ok {
		t.Fatal("Provider.Extract returned false")
	}
	evidence, ok := payload.(Evidence)
	if !ok {
		t.Fatalf("payload = %T", payload)
	}
	if evidence.RayID != "94e8ac88a48e1c5d" || evidence.Colo != "DUB" {
		t.Fatalf("ray evidence = %#v", evidence)
	}
	if evidence.VisitorScheme != "https" || evidence.Worker != "storefront.example.workers.dev" {
		t.Fatalf("request evidence = %#v", evidence)
	}
}

func TestProviderRejectsHeadersWithoutValidRay(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://api.inttegro.com/orders", nil)
	req.Header.Set("CF-Ray", "caller-controlled")
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")

	if got, ok := (Provider{}).Extract(req); ok || got != nil {
		t.Fatalf("Extract = %#v, %v", got, ok)
	}
}
