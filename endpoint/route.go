package endpoint

import (
	"fmt"
	"strings"
	"time"
)

// RoutePathMode describes how a gateway should forward an incoming route path
// to its backend.
type RoutePathMode string

const (
	// RoutePathModeAppend forwards the matched request path to the backend.
	RoutePathModeAppend RoutePathMode = "append"
	// RoutePathModeConstant forwards to the backend address without appending the
	// matched request path.
	RoutePathModeConstant RoutePathMode = "constant"
)

// RouteBackend identifies the upstream target for a transcribed endpoint.
type RouteBackend struct {
	// Address is the upstream backend base URL or provider-specific backend
	// address used by route document writers.
	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	// PathMode describes whether a generated gateway should append the matched
	// route path to Address or treat Address as the complete backend target.
	PathMode RoutePathMode `json:"path_mode,omitempty" yaml:"path_mode,omitempty"`

	// Timeout is intentionally a Go duration. Spec writers translate it into
	// their target-specific backend timeout fields.
	Timeout time.Duration `json:"-" yaml:"-"`
}

// RouteSpec describes routing/backend metadata used by spec writers.
type RouteSpec struct {
	// Backend describes where gateway-style transcribers should send matching
	// requests.
	Backend RouteBackend `json:"backend" yaml:"backend,omitempty"`
}

// WithDefaults returns a RouteSpec with empty fields filled from defaults.
func (spec RouteSpec) WithDefaults(defaults RouteSpec) RouteSpec {
	defaults = NormalizeRouteSpec(defaults)
	spec = NormalizeRouteSpec(spec)

	merged := RouteSpec{
		Backend: spec.Backend.WithDefaults(defaults.Backend)}

	return NormalizeRouteSpec(merged)
}

// WithDefaults returns a RouteBackend with empty fields filled from defaults.
func (backend RouteBackend) WithDefaults(defaults RouteBackend) RouteBackend {
	defaults = NormalizeRouteBackend(defaults)
	backend = NormalizeRouteBackend(backend)

	merged := defaults
	if backend.Address != "" {
		merged.Address = backend.Address
	}
	if backend.PathMode != "" {
		merged.PathMode = backend.PathMode
	}
	if backend.Timeout != 0 {
		merged.Timeout = backend.Timeout
	}

	return NormalizeRouteBackend(merged)
}

// NormalizeRouteSpec normalizes route backend metadata.
func NormalizeRouteSpec(spec RouteSpec) RouteSpec {
	spec.Backend = NormalizeRouteBackend(spec.Backend)

	return spec
}

// NormalizeRouteBackend trims backend metadata, normalizes its path mode, and
// panics on negative timeouts.
func NormalizeRouteBackend(backend RouteBackend) RouteBackend {
	backend.Address = strings.TrimSpace(backend.Address)
	backend.PathMode = NormalizeRoutePathMode(backend.PathMode)
	if backend.Timeout < 0 {
		panic(fmt.Sprintf("httpapi: route backend timeout cannot be negative: %s", backend.Timeout))
	}

	return backend
}

// NormalizeRoutePathMode trims and canonicalizes a route path mode.
func NormalizeRoutePathMode(mode RoutePathMode) RoutePathMode {
	switch strings.TrimSpace(strings.ToLower(string(mode))) {
	case "":
		return ""
	case string(RoutePathModeAppend):
		return RoutePathModeAppend
	case string(RoutePathModeConstant):
		return RoutePathModeConstant
	default:
		panic(fmt.Sprintf("httpapi: unsupported route path mode %q", mode))
	}
}
