// Package cloudflare extracts bounded request evidence added by Cloudflare's
// edge network. Installing Provider is an explicit assertion that the service's
// ingress makes these headers trustworthy.
package cloudflare

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/inttegro/httpapi/edge"
)

const (
	providerName = "cloudflare"
	maxChainLen  = 512
	maxWorkerLen = 253
)

// Evidence preserves Cloudflare-specific request metadata beneath the
// edge.cloudflare discriminator payload.
type Evidence struct {
	RayID          string `json:"ray_id"`
	Colo           string `json:"colo,omitempty"`
	ConnectingIP   string `json:"connecting_ip,omitempty"`
	ConnectingIPv6 string `json:"connecting_ipv6,omitempty"`
	PseudoIPv4     string `json:"pseudo_ipv4,omitempty"`
	Country        string `json:"country,omitempty"`
	VisitorScheme  string `json:"visitor_scheme,omitempty"`
	Worker         string `json:"worker,omitempty"`
	CDNLoop        string `json:"cdn_loop,omitempty"`
	EdgeWorkerVia  string `json:"edge_worker_via,omitempty"`
}

// Provider extracts Cloudflare evidence. Merely constructing a Provider does
// not change httpapi behavior; a service must install it explicitly.
type Provider struct{}

// Name returns the edge audit discriminator.
func (Provider) Name() string { return providerName }

// Extract returns evidence only when CF-Ray is syntactically valid.
func (Provider) Extract(req *http.Request) (any, bool) {
	if req == nil {
		return nil, false
	}

	rayID, colo := ray(req.Header.Get("CF-Ray"))
	if rayID == "" {
		return nil, false
	}

	return Evidence{
		RayID:          rayID,
		Colo:           colo,
		ConnectingIP:   normalizedIP(req.Header.Get("CF-Connecting-IP")),
		ConnectingIPv6: normalizedIP(req.Header.Get("CF-Connecting-IPv6")),
		PseudoIPv4:     normalizedIP(req.Header.Get("CF-Pseudo-IPv4")),
		Country:        country(req.Header.Get("CF-IPCountry")),
		VisitorScheme:  visitorScheme(req.Header.Get("CF-Visitor")),
		Worker:         bounded(req.Header.Get("CF-Worker"), maxWorkerLen),
		CDNLoop:        bounded(req.Header.Get("CDN-Loop"), maxChainLen),
		EdgeWorkerVia:  bounded(req.Header.Get("CF-EW-Via"), maxChainLen),
	}, true
}

var _ edge.Provider = Provider{}

func ray(value string) (string, string) {
	value, _, _ = strings.Cut(strings.TrimSpace(value), ",")
	rayID, colo, hasColo := strings.Cut(value, "-")
	rayID = strings.ToLower(strings.TrimSpace(rayID))
	if len(rayID) != 16 {
		return "", ""
	}
	if _, err := hex.DecodeString(rayID); err != nil {
		return "", ""
	}
	if !hasColo {
		return rayID, ""
	}
	colo = strings.ToUpper(strings.TrimSpace(colo))
	if len(colo) != 3 || !alphaNumeric(colo) {
		colo = ""
	}
	return rayID, colo
}

func country(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 || !alphaNumeric(value) {
		return ""
	}
	return value
}

func alphaNumeric(value string) bool {
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func normalizedIP(value string) string {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.String()
	}
	return ""
}

func visitorScheme(value string) string {
	var visitor struct {
		Scheme string `json:"scheme"`
	}
	if err := json.Unmarshal([]byte(value), &visitor); err != nil {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(visitor.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > limit {
		return ""
	}
	return value
}
