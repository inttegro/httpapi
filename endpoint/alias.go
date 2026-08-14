package endpoint

import "strings"

// AliasSpec describes an additional route surface for an endpoint.
//
// Aliases share the endpoint's handler, request contract, response contract,
// access policy, runtime limits, idempotency, and other behavior. OperationID is
// optional route-facing documentation metadata for the alias; when unset,
// transcribers derive it from the canonical operation ID.
type AliasSpec struct {
	// Path is the additional route pattern registered for the endpoint.
	Path string `json:"path" yaml:"path"`

	// OperationID is the optional machine-readable operation identifier for the
	// alias route in generated route documents.
	OperationID string `json:"operation_id,omitempty" yaml:"operation_id,omitempty"`
}

// Alias returns a normalized alias route specification for path.
func Alias(path string) AliasSpec {
	return normalizeAliasSpec(AliasSpec{Path: path})
}

// WithOperationID returns spec with operationID set as the route-facing alias
// operation ID used by OpenAPI transcribers.
func (spec AliasSpec) WithOperationID(operationID string) AliasSpec {
	spec.OperationID = operationID
	return normalizeAliasSpec(spec)
}

// WithAliases replaces an endpoint's alias route specifications.
func WithAliases(aliases ...AliasSpec) EndpointOption {
	aliases = normalizeAliasSpecs(aliases)
	return func(e *Endpoint) {
		e.aliases = cloneAliasSpecs(aliases)
	}
}

// Aliases returns the endpoint's normalized alias route specifications.
func (e Endpoint) Aliases() []AliasSpec {
	return cloneAliasSpecs(e.aliases)
}

func normalizeAliasSpecs(aliases []AliasSpec) []AliasSpec {
	if len(aliases) == 0 {
		return nil
	}

	normalized := make([]AliasSpec, 0, len(aliases))
	for _, alias := range aliases {
		normalized = append(normalized, normalizeAliasSpec(alias))
	}

	return normalized
}

func normalizeAliasSpec(alias AliasSpec) AliasSpec {
	alias.Path = strings.TrimSpace(alias.Path)
	alias.OperationID = strings.TrimSpace(alias.OperationID)
	if alias.Path == "" {
		panic("httpapi: endpoint alias path is required")
	}

	return alias
}

func cloneAliasSpecs(aliases []AliasSpec) []AliasSpec {
	if len(aliases) == 0 {
		return nil
	}

	cloned := make([]AliasSpec, len(aliases))
	copy(cloned, aliases)
	return cloned
}
