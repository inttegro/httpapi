package server

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/zebodotdev/httpapi/endpoint"
)

const (
	// DefaultPort is the fallback port used when Config.Port and Config.Addr are
	// empty. 8080 is the conventional application port for container runtimes
	// such as Cloud Run, Railway, Fly, and many local dev setups. It is not a
	// protocol requirement; production services should usually pass the platform
	// provided port explicitly, often through PortFromEnv("PORT", DefaultPort).
	DefaultPort = "8080"

	// DefaultReadHeaderTimeout is the default time allowed to read request
	// headers. Three seconds is intentionally short because headers should be
	// small and immediately available on healthy clients. The default reduces
	// exposure to slowloris-style connections without being so tight that normal
	// mobile or cross-region clients routinely fail.
	DefaultReadHeaderTimeout = 3 * time.Second

	// DefaultReadTimeout is the default time allowed to read the full request.
	// Five seconds is a conservative baseline for JSON APIs and small multipart
	// requests. Endpoints that intentionally accept large uploads should set an
	// explicit service or endpoint-specific timeout instead of relying on this
	// generic default.
	DefaultReadTimeout = 5 * time.Second

	// DefaultWriteTimeout is the default time allowed to write the response.
	// Fifteen seconds is deliberately generous for ordinary API responses while
	// still bounding stuck clients and broken network paths. Streaming endpoints
	// or very large downloads should opt into a more appropriate timeout.
	DefaultWriteTimeout = 15 * time.Second

	// DefaultIdleTimeout is the default keep-alive idle timeout.
	// Sixty seconds is a common HTTP server compromise: long enough for clients
	// and load balancers to reuse connections, short enough to avoid keeping idle
	// file descriptors around indefinitely. It is a pragmatic default, not a
	// correctness requirement.
	DefaultIdleTimeout = 60 * time.Second

	// DefaultMaxHeaderBytes is the default cap for request headers.
	// 64 KiB is intentionally larger than most APIs need because authenticated
	// service requests may carry signed assertions or gateway-added metadata. It
	// is still bounded so header abuse cannot grow without limit. Services with
	// small bearer-token-only requests can safely lower it; services with very
	// large identity assertions should set an explicit larger cap after measuring
	// real headers.
	DefaultMaxHeaderBytes = 64 * 1024
)

// Middleware wraps an HTTP handler.
//
// Middleware values are applied in declaration order by New and Handler. Given
// []Middleware{A, B}, a request enters A first, then B, then the underlying mux
// or handler.
type Middleware func(http.Handler) http.Handler

// Config describes the HTTP server httpapi should build.
type Config struct {
	// Addr is the full listen address. When set, Addr takes precedence over Host
	// and Port. Use Addr when the deployment target gives you a complete address
	// such as "127.0.0.1:8080" or ":8080". Most container services only provide
	// a port, so Host and Port are usually clearer than Addr.
	Addr string

	// Host is combined with Port when Addr is empty. Leave Host empty to listen
	// on every interface, which is what container runtimes normally require.
	// Set Host only when the process must bind a specific local interface, such
	// as "127.0.0.1" for a development-only listener.
	Host string

	// Port is combined with Host when Addr is empty. If empty, DefaultPort is
	// used. Production services should usually set this from their runtime's port
	// environment variable with PortFromEnv("PORT", DefaultPort) so local runs
	// still work when the variable is absent.
	Port string

	// Description describes the mounted API surface for documentation and route
	// spec generation. Runtime binding intentionally does not fill these values:
	// public URLs, document versions, gateway hosts, and default backends are API
	// contract concerns, not socket-listener concerns.
	Description Description

	// Mux is the preferred httpapi routing surface. When Handler is nil, New uses
	// Mux as the base handler. If both Handler and Mux are nil, New creates an
	// empty endpoint.Mux. Most services should provide a mux, mount endpoint
	// groups during startup, and let server.New wrap it with CORS and middleware.
	Mux *endpoint.Mux

	// Handler overrides Mux when an application needs to bring a fully custom
	// handler tree. Use this sparingly: choosing Handler means the server package
	// cannot inspect or help with endpoint.Mux registration, but it can still
	// apply CORS, middleware, and timeout defaults around the handler.
	Handler http.Handler

	// CORS applies a Cross-Origin Resource Sharing policy as the outermost
	// middleware. Preflight requests handled here do not enter Middleware, which
	// keeps browser preflight independent from authentication, request parsing,
	// idempotency, and application middleware. Leave it nil for non-browser
	// services or when CORS is handled by an upstream edge.
	CORS *CORSConfig

	// Middleware wraps the base handler in declaration order after Config.CORS.
	// Given []Middleware{A, B}, actual requests enter A, then B, then the mux or
	// custom handler. Put tracing and request-context middleware early; put
	// middleware that expects parsed auth/session state later.
	Middleware []Middleware

	// ReadHeaderTimeout bounds header reads. Zero uses
	// DefaultReadHeaderTimeout. Set this when clients, gateways, or private
	// networks have known latency characteristics that make the default too tight
	// or too loose.
	ReadHeaderTimeout time.Duration

	// ReadTimeout bounds full request reads. Zero uses DefaultReadTimeout. Use a
	// larger value for services that accept uploads or very large JSON payloads;
	// keep it low for latency-sensitive JSON APIs.
	ReadTimeout time.Duration

	// WriteTimeout bounds response writes. Zero uses DefaultWriteTimeout. Increase
	// it for streaming, downloads, or slow clients; lower it for strict
	// low-latency internal APIs.
	WriteTimeout time.Duration

	// IdleTimeout bounds idle keep-alive connections. Zero uses
	// DefaultIdleTimeout. Tune this alongside your load balancer and platform
	// connection reuse behavior so the server does not close useful connections
	// too aggressively or retain idle ones for too long.
	IdleTimeout time.Duration

	// MaxHeaderBytes caps request headers. Zero uses DefaultMaxHeaderBytes. Lower
	// it when your API only accepts small bearer-token requests; raise it only
	// after measuring real traffic, especially for signed service-auth or
	// gateway-enriched requests with larger headers.
	MaxHeaderBytes int
}

// Server is an HTTP server plus the httpapi metadata needed to describe its
// mounted endpoint surface.
//
// Server embeds net/http.Server so existing startup code can call methods such
// as ListenAndServe directly. Use DescribeOpenAPI31 or DescribeGCPAPIGateway
// when the same configured server should produce route documents.
type Server struct {
	http.Server

	config Config
	mux    *endpoint.Mux
}

// New returns a Server with httpapi defaults applied.
func New(config Config) Server {
	config = normalizeConfig(config)

	return Server{
		Server: httpServer(config),
		config: config,
		mux:    config.Mux,
	}
}

// HTTPServer returns the embedded standard library server.
func (srv Server) HTTPServer() http.Server {
	return srv.Server
}

// Config returns the server configuration used to build srv.
func (srv Server) Config() Config {
	return srv.config
}

// Mux returns the endpoint mux used for route registration and documentation.
func (srv Server) Mux() *endpoint.Mux {
	return srv.mux
}

// MountedEndpoints returns the endpoints mounted on the server's mux.
func (srv Server) MountedEndpoints() []endpoint.MountedEndpoint {
	if srv.mux == nil {
		return nil
	}

	return srv.mux.MountedEndpoints()
}

func httpServer(config Config) http.Server {
	return http.Server{
		Addr:              Address(config),
		Handler:           Handler(config),
		ReadHeaderTimeout: timeoutOrDefault(config.ReadHeaderTimeout, DefaultReadHeaderTimeout),
		ReadTimeout:       timeoutOrDefault(config.ReadTimeout, DefaultReadTimeout),
		WriteTimeout:      timeoutOrDefault(config.WriteTimeout, DefaultWriteTimeout),
		IdleTimeout:       timeoutOrDefault(config.IdleTimeout, DefaultIdleTimeout),
		MaxHeaderBytes:    intOrDefault(config.MaxHeaderBytes, DefaultMaxHeaderBytes),
	}
}

// Handler returns config's base handler wrapped by config's middleware chain.
func Handler(config Config) http.Handler {
	config = normalizeConfig(config)

	handler := config.Handler
	if handler == nil {
		handler = config.Mux
	}

	for _, middleware := range slices.Backward(config.Middleware) {
		if middleware != nil {
			handler = middleware(handler)
		}
	}
	if config.CORS != nil {
		handler = CORSMiddleware(*config.CORS)(handler)
	}

	return handler
}

// Address returns the listen address described by config.
func Address(config Config) string {
	if addr := strings.TrimSpace(config.Addr); addr != "" {
		return addr
	}

	port := strings.TrimSpace(config.Port)
	if port == "" {
		port = DefaultPort
	}

	host := strings.TrimSpace(config.Host)
	if host == "" {
		return fmt.Sprintf(":%s", port)
	}

	return fmt.Sprintf("%s:%s", host, port)
}

// PortFromEnv returns the trimmed environment variable value or fallback when
// the variable is unset.
func PortFromEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

// ListenAndServe builds a server from config and starts it.
func ListenAndServe(config Config) error {
	srv := New(config)
	return srv.ListenAndServe()
}

// Serve starts srv.
func Serve(srv Server) error {
	return srv.ListenAndServe()
}

func normalizeConfig(config Config) Config {
	if config.Handler == nil && config.Mux == nil {
		config.Mux = endpoint.NewMux()
	}

	return config
}

func timeoutOrDefault(value, fallback time.Duration) time.Duration {
	if value != 0 {
		return value
	}
	return fallback
}

func intOrDefault(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}
