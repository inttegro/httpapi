package endpoint

import (
	parampkg "github.com/zebodotdev/httpapi/param"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

// RequestHandler is an endpoint handler that receives a parsed request body.
type RequestHandler[T any] func(r *Req, params T)

// RequestResponder is the preferred endpoint responder that receives a parsed
// request body and returns the response to render.
type RequestResponder[T any] func(r *Req, params T) *responsepkg.Res

// JSONEndpointSpec defines an endpoint whose JSON request contract is also its
// runtime parser.
type JSONEndpointSpec[T any] struct {
	// Method is the HTTP method accepted by the endpoint. Empty and unsupported
	// values panic during endpoint construction.
	Method HttpMethod

	// Path is the route pattern registered on the Go ServeMux and later exposed
	// to transcribers.
	Path string

	// Aliases are additional route patterns for the same endpoint behavior.
	// They do not duplicate handlers, request/response contracts, access policy,
	// idempotency, limits, timeouts, or other runtime behavior.
	Aliases []AliasSpec

	// Request is the JSON request parser and documentation contract.
	Request *parampkg.Request[T]

	// Handler is the compatibility function invoked after Request successfully
	// parses the endpoint request.
	Handler RequestHandler[T]

	// Respond is the preferred return-style function invoked after Request
	// successfully parses the endpoint request. Handler and Respond are mutually
	// exclusive; endpoint construction panics when both are set.
	Respond RequestResponder[T]

	// Accepts is the primary request content type. It defaults to
	// ApplicationJson.
	Accepts ContentType

	// AcceptsAny declares additional accepted request content types. Duplicate
	// entries are removed after normalization.
	AcceptsAny []ContentType

	// Access declares whether the endpoint is internal, which session kind it
	// requires, and which application-defined callers may invoke it.
	Access EndpointAccessSpec

	// Idempotency declares whether successful responses should be reserved and
	// replayed for repeated idempotency keys.
	Idempotency EndpointIdempotencySpec

	// Operation carries provider-neutral metadata for generated docs,
	// completion events, and accounting.
	Operation OperationSpec

	// Route carries provider-neutral routing/backend metadata for generated
	// route documents.
	Route RouteSpec

	// Priority captures the operational importance of the endpoint.
	Priority EndpointPriority

	// Timeout declares in-process read, handler, and write deadlines for this
	// endpoint.
	Timeout EndpointTimeoutSpec

	// Limits declares in-process request limits for this endpoint. MaxRequestBytes
	// caps the full parsed request envelope, including the request line, headers,
	// and body.
	Limits EndpointLimitsSpec

	// Controls declares app/organization control policy required before the
	// endpoint handler can run.
	Controls []EndpointControlSpec

	// Responses describes the response payloads emitted by the endpoint.
	Responses []ResponseContract

	// TimeoutHandler renders the response when the handler context reaches its
	// timeout before the endpoint produces a response. When unset, httpapi
	// renders DefaultEndpointTimeoutHandler.
	TimeoutHandler EndpointTimeoutHandler

	// AuthKeys is an optional arbitrary key set for service-specific
	// authorization metadata. httpapi clones the map before storing it.
	AuthKeys map[string]bool
}

// HandlerWithRequest parses request with httpapi/param and invokes handler
// with the parsed request value. Parse errors are rendered with the standard
// response error envelope.
func HandlerWithRequest[T any](
	request *parampkg.Request[T],
	handler RequestHandler[T],
) Handler {
	if request == nil {
		panic("httpapi: endpoint request parser is required")
	}
	if handler == nil {
		panic("httpapi: endpoint request handler is required")
	}

	return func(r *Req) {
		params, err := request.Parse(r)
		if err != nil {
			responsepkg.RenderParamError(r, err)
			return
		}

		handler(r, params)
	}
}

// HandlerWithRequestResponder parses request with httpapi/param and invokes
// responder with the parsed request value. Parse errors are returned through
// the standard response error envelope.
func HandlerWithRequestResponder[T any](
	request *parampkg.Request[T],
	responder RequestResponder[T],
) Handler {
	if request == nil {
		panic("httpapi: endpoint request parser is required")
	}
	if responder == nil {
		panic("httpapi: endpoint request responder is required")
	}

	return HandlerFromResponder(func(r *Req) *responsepkg.Res {
		params, err := request.Parse(r)
		if err != nil {
			return responsepkg.ParamError(err)
		}

		return responder(r, params)
	})
}

// DefineJSONEndpoint returns an endpoint whose JSON request parser is used for
// both runtime request parsing and request-body documentation.
func DefineJSONEndpoint[T any](spec JSONEndpointSpec[T]) Endpoint {
	return DefineEndpoint(EndpointSpec{
		Method:         spec.Method,
		Path:           spec.Path,
		Aliases:        spec.Aliases,
		Handler:        handlerFromJSONEndpointSpec(spec),
		Accepts:        spec.Accepts,
		AcceptsAny:     spec.AcceptsAny,
		Access:         spec.Access,
		Idempotency:    spec.Idempotency,
		Operation:      spec.Operation,
		Route:          spec.Route,
		Priority:       spec.Priority,
		Timeout:        spec.Timeout,
		Limits:         spec.Limits,
		Controls:       spec.Controls,
		Request:        RequestBody(spec.Request),
		Responses:      spec.Responses,
		TimeoutHandler: spec.TimeoutHandler,
		AuthKeys:       spec.AuthKeys,
	})
}

func handlerFromJSONEndpointSpec[T any](spec JSONEndpointSpec[T]) Handler {
	if spec.Handler != nil && spec.Respond != nil {
		panic("httpapi: json endpoint spec cannot set both Handler and Respond")
	}
	if spec.Respond != nil {
		return HandlerWithRequestResponder(spec.Request, spec.Respond)
	}

	return HandlerWithRequest(spec.Request, spec.Handler)
}
