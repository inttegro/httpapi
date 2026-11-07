// Package edge defines the optional HTTP edge-provider extension point used by
// request audits. Provider implementations live in subpackages and are never
// enabled implicitly.
package edge

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
)

var providerNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Provider extracts provider-specific request evidence. Name is both the
// discriminator and the JSON property that contains the returned payload.
// Implementations must return bounded, audit-safe values.
type Provider interface {
	Name() string
	Extract(*http.Request) (any, bool)
}

// Audit is a discriminated provider envelope. For example, a provider named
// "cloudflare" serializes its evidence beneath the "cloudflare" property.
type Audit struct {
	provider string
	payload  any
}

// Extract asks provider for evidence and returns a validated audit envelope.
func Extract(provider Provider, req *http.Request) *Audit {
	if provider == nil || req == nil {
		return nil
	}
	name := provider.Name()
	if !providerNamePattern.MatchString(name) {
		return nil
	}

	payload, ok := provider.Extract(req)
	if !ok || payload == nil {
		return nil
	}

	return &Audit{provider: name, payload: payload}
}

// Provider returns the audit discriminator.
func (a *Audit) Provider() string {
	if a == nil {
		return ""
	}
	return a.provider
}

// MarshalJSON serializes the provider discriminator and its matching payload.
func (a Audit) MarshalJSON() ([]byte, error) {
	if !providerNamePattern.MatchString(a.provider) || a.payload == nil {
		return nil, errors.New("httpapi/edge: invalid audit envelope")
	}

	return json.Marshal(map[string]any{
		"provider": a.provider,
		a.provider: a.payload,
	})
}
