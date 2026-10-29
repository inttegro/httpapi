package param

import (
	"context"

	callerpkg "github.com/inttegro/httpapi/caller"
)

const requestValueKey = "httpapi.request"

// Option configures a parse call.
type Option func(*parseConfig)

// CallerSource exposes the caller attached to a parsed request.
//
// request.Req implements this interface. Keeping the dependency as a tiny
// interface lets param use the request caller without importing the request
// package.
type CallerSource interface {
	// RequestCaller returns the application-defined request caller.
	RequestCaller() callerpkg.Caller
}

// RequestSource exposes the request fields param needs when parsing an httpapi
// request directly.
//
// request.Req implements this interface. The dependency stays as a small
// interface so param can consume request state without importing the request
// package and creating a package cycle.
type RequestSource interface {
	CallerSource

	// Context returns the request context used by object parsers.
	Context() context.Context

	// RequestBody returns the buffered request body bytes to parse.
	RequestBody() []byte
}

type parseConfig struct {
	caller callerpkg.Caller
	ctx    context.Context
	values map[string]any
}

// WithCaller attaches caller information used by parameter availability rules.
//
// Use WithCaller in tests or when parsing outside an httpapi request. Endpoint
// handlers that receive *request.Req or *endpoint.Req should usually pass the
// request directly to Request.Parse.
func WithCaller(caller callerpkg.Caller) Option {
	return func(config *parseConfig) {
		config.caller = caller
	}
}

// WithRequestCaller attaches caller information from source.
//
// This is the request-bound form of WithCaller. It keeps request parsing aligned
// with endpoint availability and response projection by reading the same caller
// stored on the request.
func WithRequestCaller(source CallerSource) Option {
	return func(config *parseConfig) {
		if source == nil {
			return
		}
		config.caller = source.RequestCaller()
	}
}

// WithRequest attaches request context for a parse call.
//
// Request.Parse calls WithRequest automatically when input implements
// RequestSource. Use this option only when the JSON body is supplied separately
// but object parsers still need access to the request's caller, context, or
// request object.
func WithRequest(source RequestSource) Option {
	return func(config *parseConfig) {
		if source == nil {
			return
		}
		config.caller = source.RequestCaller()
		config.ctx = source.Context()
		withValue(config, requestValueKey, source)
	}
}

// WithContext attaches a context that object parsers can read from Values.
func WithContext(ctx context.Context) Option {
	return func(config *parseConfig) {
		config.ctx = ctx
	}
}

// WithValue attaches application-defined parse context.
func WithValue(key string, value any) Option {
	return func(config *parseConfig) {
		withValue(config, key, value)
	}
}

// RequestFromValues returns the request source attached to Values by
// Request.Parse or WithRequest.
//
// It is intended for final object parsers that need request-derived fields,
// such as an authenticated application id, without asking endpoint code to pass
// a service-specific value key by hand.
func RequestFromValues[T RequestSource](values Values) (T, bool) {
	raw, ok := values.Value(requestValueKey)
	if !ok {
		return zeroValue[T](), false
	}
	source, ok := raw.(T)
	if !ok {
		return zeroValue[T](), false
	}
	return source, true
}

func newParseConfig(options []Option) parseConfig {
	config := parseConfig{ctx: context.Background()}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if config.ctx == nil {
		config.ctx = context.Background()
	}
	return config
}

func withValue(config *parseConfig, key string, value any) {
	if config == nil || key == "" {
		return
	}
	if config.values == nil {
		config.values = map[string]any{}
	}
	config.values[key] = value
}
