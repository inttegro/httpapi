package endpoint

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Mux is httpapi's default endpoint router.
//
// Mux wraps a standard library ServeMux, mounts EndpointGroup values with their
// inherited metadata applied, detects duplicate method/path registrations before
// mutating its registry, and keeps a read-only snapshot of mounted endpoints for
// documentation and operational tooling.
type Mux struct {
	serveMux *http.ServeMux
	routes   map[string]MountedEndpoint
	mounted  []MountedEndpoint
	groups   []EndpointGroup
}

// MuxOption configures a Mux during construction.
type MuxOption func(*Mux)

var (
	// ErrNilMux reports that a nil Mux receiver was asked to mount endpoints.
	ErrNilMux = errors.New("httpapi: endpoint mux is nil")

	// ErrNilServeMux reports that a nil standard library mux was passed to the
	// compatibility EndpointGroup.Mount method.
	ErrNilServeMux = errors.New("httpapi: serve mux is nil")
)

// MountedEndpoint describes one endpoint registered on a Mux.
type MountedEndpoint struct {
	// Method is the normalized HTTP method registered with the underlying mux.
	Method HttpMethod

	// Path is the full mounted path after group prefixes have been applied.
	Path string

	// Endpoint is the mounted endpoint with inherited group metadata applied.
	Endpoint Endpoint

	// Alias is set when Path is an alias route for Endpoint.
	Alias *AliasSpec
}

// ErrDuplicateMuxRoute reports that a method/path pair was mounted more than
// once.
type ErrDuplicateMuxRoute struct {
	Method HttpMethod
	Path   string
}

// Error returns the duplicate route error message.
func (err ErrDuplicateMuxRoute) Error() string {
	return fmt.Sprintf("httpapi: duplicate endpoint route %s %s", err.Method, err.Path)
}

// NewMux returns httpapi's default endpoint router.
func NewMux(options ...MuxOption) *Mux {
	mux := &Mux{
		serveMux: http.NewServeMux(),
		routes:   map[string]MountedEndpoint{},
	}
	for _, option := range options {
		if option != nil {
			option(mux)
		}
	}
	if mux.serveMux == nil {
		mux.serveMux = http.NewServeMux()
	}
	if mux.routes == nil {
		mux.routes = map[string]MountedEndpoint{}
	}

	return mux
}

// WithServeMux configures Mux to register handlers on serveMux.
//
// Use this when an application already owns a standard library mux but still
// wants httpapi's group mounting, duplicate checks, and endpoint registry.
func WithServeMux(serveMux *http.ServeMux) MuxOption {
	return func(mux *Mux) {
		if mux != nil && serveMux != nil {
			mux.serveMux = serveMux
		}
	}
}

// ServeHTTP implements http.Handler.
func (mux *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux.httpMux().ServeHTTP(w, r)
}

// Handler returns the underlying HTTP handler.
func (mux *Mux) Handler() http.Handler {
	return mux.httpMux()
}

// ServeMux returns the underlying standard library mux.
func (mux *Mux) ServeMux() *http.ServeMux {
	return mux.httpMux()
}

// Mount registers all endpoints in groups.
//
// Mount validates the full batch before registering handlers. If it returns an
// error, none of the endpoints from this call have been mounted.
func (mux *Mux) Mount(groups ...EndpointGroup) error {
	if mux == nil {
		return ErrNilMux
	}

	mounted, err := mux.prepareGroups(groups...)
	if err != nil {
		return err
	}

	mux.registerPrepared(mounted, groups...)
	return nil
}

// MustMount registers groups and panics if any route cannot be mounted.
func (mux *Mux) MustMount(groups ...EndpointGroup) {
	if err := mux.Mount(groups...); err != nil {
		panic(err)
	}
}

// MountEndpoints registers endpoints without applying a group path prefix.
func (mux *Mux) MountEndpoints(endpoints ...Endpoint) error {
	group := EndpointGroup{Endpoints: append([]Endpoint(nil), endpoints...)}
	return mux.Mount(group)
}

// MustMountEndpoints registers endpoints without applying a group path prefix
// and panics if any route cannot be mounted.
func (mux *Mux) MustMountEndpoints(endpoints ...Endpoint) {
	if err := mux.MountEndpoints(endpoints...); err != nil {
		panic(err)
	}
}

// MountedEndpoints returns mounted endpoints in registration order.
func (mux *Mux) MountedEndpoints() []MountedEndpoint {
	if mux == nil || len(mux.mounted) == 0 {
		return nil
	}

	mounted := make([]MountedEndpoint, 0, len(mux.mounted))
	for _, endpoint := range mux.mounted {
		mounted = append(mounted, cloneMountedEndpoint(endpoint))
	}
	return mounted
}

// Groups returns the endpoint groups mounted through this mux.
func (mux *Mux) Groups() []EndpointGroup {
	if mux == nil || len(mux.groups) == 0 {
		return nil
	}

	return append([]EndpointGroup(nil), mux.groups...)
}

func (mux *Mux) prepareGroups(groups ...EndpointGroup) ([]MountedEndpoint, error) {
	prepared := []MountedEndpoint{}
	pending := map[string]MountedEndpoint{}
	for _, group := range groups {
		for _, mounted := range mountedEndpointsForGroup(group) {
			key := muxRouteKey(mounted.Method, mounted.Path)
			if _, ok := mux.routes[key]; ok {
				return nil, ErrDuplicateMuxRoute{Method: mounted.Method, Path: mounted.Path}
			}
			if _, ok := pending[key]; ok {
				return nil, ErrDuplicateMuxRoute{Method: mounted.Method, Path: mounted.Path}
			}

			pending[key] = mounted
			prepared = append(prepared, mounted)
		}
	}

	return prepared, nil
}

func (mux *Mux) registerPrepared(mounted []MountedEndpoint, groups ...EndpointGroup) {
	mux.ensure()
	for _, mounted := range mounted {
		logr.Printf(
			"attaching endpoint to multiplexer:"+
				" method=%s path=%s accepts=%s",
			mounted.Method, mounted.Path,
			joinContentTypes(mounted.Endpoint.accepts),
		)

		mux.serveMux.HandleFunc(
			fmt.Sprintf("%s %s", mounted.Method, mounted.Path),
			mounted.Endpoint.Handler(),
		)
		mux.routes[muxRouteKey(mounted.Method, mounted.Path)] = mounted
		mux.mounted = append(mux.mounted, mounted)
	}
	mux.groups = append(mux.groups, groups...)
}

func (mux *Mux) httpMux() *http.ServeMux {
	if mux == nil {
		return http.NewServeMux()
	}
	mux.ensure()
	return mux.serveMux
}

func (mux *Mux) ensure() {
	if mux.serveMux == nil {
		mux.serveMux = http.NewServeMux()
	}
	if mux.routes == nil {
		mux.routes = map[string]MountedEndpoint{}
	}
}

func mountedEndpointsForGroup(group EndpointGroup) []MountedEndpoint {
	resolved := group.ResolvedEndpoints()
	if len(resolved) == 0 {
		return nil
	}

	mounted := make([]MountedEndpoint, 0, len(resolved))
	for _, endpoint := range resolved {
		path := mountedEndpointPath(group.PathPrefix, endpoint.pattern)
		mounted = append(mounted, MountedEndpoint{
			Method:   endpoint.method,
			Path:     path,
			Endpoint: endpoint,
		})
		for _, alias := range endpoint.Aliases() {
			aliasPath := mountedEndpointPath(group.PathPrefix, alias.Path)
			alias := alias
			mounted = append(mounted, MountedEndpoint{
				Method:   endpoint.method,
				Path:     aliasPath,
				Endpoint: endpoint,
				Alias:    &alias,
			})
		}
	}

	return mounted
}

func mountedEndpointPath(prefix, pattern string) string {
	path, err := url.JoinPath(prefix, pattern)
	if err != nil {
		panic(fmt.Sprintf("httpapi: invalid endpoint path prefix=%q path=%q: %v", prefix, pattern, err))
	}
	path, err = url.PathUnescape(path)
	if err != nil {
		panic(fmt.Sprintf("httpapi: invalid escaped endpoint path prefix=%q path=%q: %v", prefix, pattern, err))
	}
	if path == "" {
		return "/"
	}
	if path[0] != '/' {
		return "/" + path
	}

	return path
}

func muxRouteKey(method HttpMethod, path string) string {
	return fmt.Sprintf("%s %s", method, path)
}

func cloneMountedEndpoint(mounted MountedEndpoint) MountedEndpoint {
	if mounted.Alias == nil {
		return mounted
	}
	alias := *mounted.Alias
	mounted.Alias = &alias
	return mounted
}
