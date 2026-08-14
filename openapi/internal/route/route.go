package route

import (
	"fmt"
	"net/url"
	"strings"

	endpointpkg "github.com/zebodotdev/httpapi/endpoint"
)

// Route is a resolved endpoint mount used by OpenAPI transcribers.
type Route struct {
	// Method is the resolved endpoint HTTP method.
	Method endpointpkg.HttpMethod

	// Path is the resolved path after joining group and transcriber prefixes.
	Path string

	// CanonicalPath is the resolved canonical endpoint path for this route. For
	// canonical routes it is equal to Path; for aliases it remains the endpoint's
	// canonical path.
	CanonicalPath string

	// Endpoint is the endpoint whose metadata produced this route.
	Endpoint endpointpkg.Endpoint

	// Alias is set when Path is an alias route for Endpoint.
	Alias *endpointpkg.AliasSpec
}

// Routes is an ordered list of resolved endpoint mounts.
type Routes []Route

// FromEndpoint returns the route for one endpoint without a group prefix.
func FromEndpoint(endpoint endpointpkg.Endpoint) (Routes, error) {
	return fromEndpoint("", endpoint)
}

// FromGroup returns resolved endpoint routes for a group.
//
// Group defaults are applied before routes are returned.
func FromGroup(group endpointpkg.EndpointGroup) (Routes, error) {
	endpoints := group.ResolvedEndpoints()
	if len(endpoints) == 0 {
		return nil, nil
	}

	routes := make(Routes, 0, len(endpoints))
	for _, endpoint := range endpoints {
		endpointRoutes, err := fromEndpoint(group.PathPrefix, endpoint)
		if err != nil {
			return nil, err
		}
		routes = append(routes, endpointRoutes...)
	}

	return routes, nil
}

// FromGroups returns resolved endpoint routes for several groups while
// preserving group and endpoint declaration order.
func FromGroups(groups ...endpointpkg.EndpointGroup) (Routes, error) {
	var routes Routes
	for _, group := range groups {
		groupRoutes, err := FromGroup(group)
		if err != nil {
			return nil, err
		}
		routes = append(routes, groupRoutes...)
	}

	return routes, nil
}

// WithPathPrefix returns a copy of the routes with prefix applied to every path.
func (routes Routes) WithPathPrefix(prefix string) (Routes, error) {
	if len(routes) == 0 {
		return nil, nil
	}

	prefixed := make(Routes, 0, len(routes))
	for _, route := range routes {
		canonicalPath := route.CanonicalPath
		if canonicalPath == "" {
			canonicalPath = route.Path
		}

		path, err := JoinPath(prefix, route.Path)
		if err != nil {
			return nil, err
		}
		canonicalPath, err = JoinPath(prefix, canonicalPath)
		if err != nil {
			return nil, err
		}

		route.Path = path
		route.CanonicalPath = canonicalPath
		prefixed = append(prefixed, route)
	}

	return prefixed, nil
}

func fromEndpoint(prefix string, endpoint endpointpkg.Endpoint) (Routes, error) {
	path, err := JoinPath(prefix, endpoint.Pattern())
	if err != nil {
		return nil, err
	}

	routes := Routes{{
		Method:        endpoint.Method(),
		Path:          path,
		CanonicalPath: path,
		Endpoint:      endpoint,
	}}
	for _, alias := range endpoint.Aliases() {
		path, err := JoinPath(prefix, alias.Path)
		if err != nil {
			return nil, err
		}
		alias := alias
		routes = append(routes, Route{
			Method:        endpoint.Method(),
			Path:          path,
			CanonicalPath: routes[0].CanonicalPath,
			Endpoint:      endpoint,
			Alias:         &alias,
		})
	}

	return routes, nil
}

// JoinPath joins route path fragments using URL path semantics and returns a
// normalized path suitable for OpenAPI transcription.
func JoinPath(prefix, pattern string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	pattern = strings.TrimSpace(pattern)
	if prefix == "" {
		prefix = "/"
	}
	if pattern == "" {
		pattern = "/"
	}

	path, err := url.JoinPath(prefix, pattern)
	if err != nil {
		return "", fmt.Errorf("openapi/internal/route: join path: %w", err)
	}
	path, err = url.PathUnescape(path)
	if err != nil {
		return "", fmt.Errorf("openapi/internal/route: unescape path: %w", err)
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}

	return path, nil
}
