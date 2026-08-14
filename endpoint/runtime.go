package endpoint

import (
	"log"
	"net/http"
	"os"
	"time"

	callerpkg "github.com/zebodotdev/httpapi/caller"
	e "github.com/zebodotdev/httpapi/erreur"
	requestpkg "github.com/zebodotdev/httpapi/request"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

// HttpMethod is the HTTP method type accepted by endpoint constructors.
type HttpMethod = Method

// Handler is the compatibility application handler signature used by httpapi
// endpoints.
//
// The handler receives a parsed Req and should set exactly one response through
// response.RenderJSON, response.RenderErr, response.RenderStream, or
// Req.SetResponse.
type Handler func(r *Req)

// Responder is the preferred application handler signature used by httpapi
// endpoints.
//
// The responder receives a parsed Req and returns the response to render.
type Responder func(r *Req) *responsepkg.Res

// Req is the safe request type passed to endpoint handlers.
type Req = requestpkg.Req

// Res is the response type produced by endpoint handlers.
type Res = responsepkg.Res

const (
	// APIRequestsTable is the default audit table name used by the request
	// package.
	APIRequestsTable = requestpkg.ReqTable

	contentTypeHeaderKey = "content-type"

	// TAG is the log prefix used by the endpoint runtime.
	TAG = "[httpapi/endpoint]: "
)

var logr = log.New(os.Stdout, TAG, log.Flags()|log.Llongfile)

// Endpoint couples route metadata, access policy, runtime behavior, and the
// application handler for one HTTP operation.
type Endpoint struct {
	accepts           []ContentType
	method            HttpMethod
	pattern           string
	aliases           []AliasSpec
	handler           http.HandlerFunc
	rawHandler        Handler
	idempotent        bool
	resolver          IdempotencyScopeResolver
	access            endpointAccessPolicy
	operation         endpointOperationPolicy
	route             RouteSpec
	priority          endpointPriorityPolicy
	timeout           endpointTimeoutPolicy
	limits            endpointLimitsPolicy
	requestContract   RequestContract
	responseContracts []ResponseContract
	authKeys          map[string]bool
}

// Handler returns the net/http handler built around the application handler.
func (e Endpoint) Handler() http.HandlerFunc { return e.handler }

// Method returns the normalized HTTP method accepted by the endpoint.
func (e Endpoint) Method() HttpMethod { return e.method }

// Pattern returns the endpoint path pattern before any group prefix is applied.
func (e Endpoint) Pattern() string { return e.pattern }

// Accepts returns the primary request content type accepted by the endpoint.
func (e Endpoint) Accepts() ContentType { return primaryContentType(e.accepts) }

// AcceptedContentTypes returns a copy of every request content type accepted by
// the endpoint.
func (e Endpoint) AcceptedContentTypes() []ContentType {
	return cloneContentTypes(e.accepts)
}

// IsInternal reports whether the endpoint is callable only by service sessions.
func (e Endpoint) IsInternal() bool { return e.accessPolicy().internal }

// IsIdempotent reports whether the endpoint enforces idempotency keys and
// replay.
func (e Endpoint) IsIdempotent() bool { return e.idempotent }

// Authorization returns the normalized authorization requirement for the
// endpoint.
func (e Endpoint) Authorization() AuthorizationRequirement {
	return e.accessPolicy().auth
}

// RequiresAuthorization reports whether the endpoint requires any
// authenticated session before its handler may run.
func (e Endpoint) RequiresAuthorization() bool { return e.Authorization().Required }

// RouteSpec returns provider-neutral routing/backend metadata for spec
// transcribers.
func (e Endpoint) RouteSpec() RouteSpec { return e.routeSpec() }

// Priority returns the normalized operational priority for the endpoint.
func (e Endpoint) Priority() EndpointPriority { return e.priorityPolicy().priority }

// AuthKeys returns a copy of endpoint authorization metadata keys.
func (e Endpoint) AuthKeys() map[string]bool { return cloneEndpointAuthKeys(e.authKeys) }

// CallerAvailability returns the caller set allowed to invoke the endpoint.
func (e Endpoint) CallerAvailability() callerpkg.Set { return e.callerAvailability() }

// AvailableCallers returns the endpoint's allowed callers in definition order.
func (e Endpoint) AvailableCallers() []callerpkg.Caller {
	return e.CallerAvailability().Callers()
}

// AvailableTo reports whether caller may invoke the endpoint.
func (e Endpoint) AvailableTo(caller callerpkg.Caller) bool {
	return e.CallerAvailability().Allows(caller)
}

// EndpointGroup groups endpoints together under one path prefix and applies
// shared defaults to each endpoint.
type EndpointGroup struct {
	// PathPrefix is prepended to every endpoint pattern when the group is
	// mounted or transcribed.
	PathPrefix string

	// Endpoints is the ordered list of endpoint definitions in the group.
	Endpoints []Endpoint

	// Internal marks every endpoint in the group as service-only unless a future
	// endpoint policy says otherwise.
	Internal bool

	// Auth is the default authorization requirement inherited by endpoints that
	// do not declare their own requirement.
	Auth AuthorizationRequirement

	// Callers restricts every endpoint in the group to these application-defined
	// caller labels. Endpoint-level caller availability can narrow this set but
	// cannot widen it. Leave it empty to make the group available to every
	// caller.
	Callers []callerpkg.Caller

	// Operation is the default operation metadata inherited by endpoints.
	// Operation.ID is intentionally not inherited because it must be unique per
	// operation. Summary and Accounting inherit when the endpoint leaves them
	// unset.
	Operation OperationSpec

	// Route is the default routing/backend metadata inherited by endpoint
	// RouteSpec values.
	Route RouteSpec

	// Priority is the default operational priority inherited by endpoints
	// without their own priority.
	Priority EndpointPriority

	// Timeout is the default runtime timeout budget inherited field-by-field by
	// endpoints without their own timeout values.
	Timeout EndpointTimeoutSpec

	// Limits is the default runtime request limit inherited field-by-field by
	// endpoints without their own limit values.
	Limits EndpointLimitsSpec

	// TimeoutHandler is the default response renderer for endpoint handler
	// timeouts in this group.
	TimeoutHandler EndpointTimeoutHandler
}

// Authorization returns the normalized default authorization requirement for
// endpoints in the group.
func (eg EndpointGroup) Authorization() AuthorizationRequirement {
	return normalizeAuthorizationRequirement(eg.Auth)
}

// RequiresAuthorization reports whether the group default requires
// authentication.
func (eg EndpointGroup) RequiresAuthorization() bool { return eg.Auth.Required }

// RouteSpec returns the group's default routing/backend metadata.
func (eg EndpointGroup) RouteSpec() RouteSpec { return eg.Route }

// ResolvedEndpoints returns endpoints with group-level metadata applied.
// Transcribers and other read-only consumers should use this instead of
// inspecting Endpoints directly when group defaults matter.
func (eg EndpointGroup) ResolvedEndpoints() []Endpoint {
	if len(eg.Endpoints) == 0 {
		return nil
	}
	endpoints := make([]Endpoint, 0, len(eg.Endpoints))
	for _, endpoint := range eg.Endpoints {
		endpoints = append(endpoints, eg.endpointWithGroupMetadata(endpoint))
	}
	return endpoints
}

// Add adds a new endpoint to the group after applying group defaults.
//
// Add does not reject duplicate method/path pairs. If duplicates are mounted on
// the same ServeMux, the mux's own duplicate handling determines the outcome.
func (eg *EndpointGroup) Add(e Endpoint) {
	eg.Endpoints = append(eg.Endpoints, eg.endpointWithGroupMetadata(e))
}

// Mount attaches all endpoints in the group to the given
// multiplexer. It prefixes the endpoint's pattern with the
// group's prefix so that an endpoint that was originally defined
// as, say, `/endpoint`, in a group with `group` prefix eventually
// becomes `/group/endpoint`.
func (eg *EndpointGroup) Mount(mux *http.ServeMux) {
	if mux == nil {
		panic(ErrNilServeMux)
	}

	NewMux(WithServeMux(mux)).MustMount(*eg)
}

// NewEndpoint returns a basic endpoint from positional constructor arguments.
//
// Prefer DefineEndpoint for new code. NewEndpoint remains useful for older
// call sites that configure metadata with EndpointOption values.
func NewEndpoint(
	meth HttpMethod,
	pattern string,
	handler Handler,
	opts ...EndpointOption,
) Endpoint {
	return defineEndpointWithOptions(EndpointSpec{
		Method:  meth,
		Path:    pattern,
		Handler: handler,
	}, opts...)
}

// HandlerFromResponder adapts a return-style responder to a compatibility
// Handler.
//
// If responder returns nil after already rendering a response on Req, that
// response is preserved. If it returns nil without rendering, httpapi renders a
// standard unexpected-error response.
func HandlerFromResponder(responder Responder) Handler {
	if responder == nil {
		panic("httpapi: endpoint responder is required")
	}

	return func(r *Req) {
		res := responder(r)
		if res != nil {
			responsepkg.Render(r, res)
			return
		}
		if r != nil && r.Response() != nil {
			return
		}

		responsepkg.RenderErr(r, e.Unexpected())
	}
}

// NewIdempotentEndpoint returns an endpoint with idempotency enabled.
//
// Prefer DefineEndpoint with EndpointSpec.Idempotency for new code.
func NewIdempotentEndpoint(
	meth HttpMethod,
	pattern string,
	handler Handler,
	opts ...EndpointOption,
) Endpoint {
	return defineEndpointWithOptions(EndpointSpec{
		Method:  meth,
		Path:    pattern,
		Handler: handler,
		Idempotency: EndpointIdempotencySpec{
			Enabled: true,
		},
	}, opts...)
}

// NewIdempotentEndpointWithScopeResolver returns an idempotent endpoint whose
// idempotency scope is computed from the request.
//
// Prefer DefineEndpoint with EndpointSpec.Idempotency.ScopeResolver for new
// code.
func NewIdempotentEndpointWithScopeResolver(
	meth HttpMethod,
	pattern string,
	resolver IdempotencyScopeResolver,
	handler Handler,
	opts ...EndpointOption,
) Endpoint {
	return defineEndpointWithOptions(EndpointSpec{
		Method:  meth,
		Path:    pattern,
		Handler: handler,
		Idempotency: EndpointIdempotencySpec{
			Enabled:       true,
			ScopeResolver: resolver,
		},
	}, opts...)
}

func (endpoint Endpoint) withRebuiltHandler() Endpoint {
	endpoint.handler = endpoint.httpHandler()
	return endpoint
}

func (endpoint Endpoint) httpHandler() http.HandlerFunc {
	// Close over the endpoint metadata so the wrapper can enforce method,
	// content type, access policy, idempotency, timeouts, and audit capture
	// before and after application handler execution.
	return func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		auditContext := r.Context()
		timeout := endpoint.TimeoutSpec()
		r, cancelTimeout := requestWithEndpointHandlerTimeout(r, timeout, startedAt)
		if cancelTimeout != nil {
			defer cancelTimeout()
		}
		responseSizeBytes := 0
		completionState := endpointCompletionState{
			startedAt: startedAt,
		}
		req := &Req{
			AppID:  requestpkg.UnauthorizedAppID,
			RecdAt: startedAt,
			Req:    r,
			ID:     requestpkg.NewID(),
		}
		defer func() {
			completedAt := time.Now()
			panicValue := recover()
			completionState.panicValue = panicValue

			req.RecdAt = startedAt
			req.Dur = completedAt.Sub(startedAt)
			if req.Res != nil {
				req.Res.SentAt = completedAt
			} else if panicValue != nil {
				responsepkg.RenderErr(req, e.Unexpected())
			}

			if err := currentAuditSink().RecordRequest(auditContext, req); err != nil {
				logr.Printf(
					"attempt to record request audit failed"+
						" request_id=%s error=%v",
					req.ID,
					err,
				)
			}

			completionState.responseSizeBytes = responseSizeBytes
			if err := endpoint.completeEndpoint(auditContext, req, completedAt, completionState); err != nil {
				logr.Printf(
					"attempt to record endpoint completion failed"+
						" request_id=%s error=%v",
					req.ID,
					err,
				)
			}

			logr.Printf(
				"completed request handling:"+
					" request_method=%s request_path=%s request_id=%s"+
					" duration=%v response_size_bytes=%d",
				req.Method(), req.Path(), req.ID,
				time.Since(startedAt), responseSizeBytes,
			)

			if panicValue != nil {
				panic(panicValue)
			}
		}()

		if r.Method != endpoint.method {
			completionState.outcome = CompletionOutcomeMethodNotAllowed
			responsepkg.RenderErr(req, e.MethodNotAllowed(endpoint.method, r.Method))
			req.Dur = time.Since(startedAt)
			written, err := writeRenderedResponse(w, req, timeout)
			responseSizeBytes = written
			if err != nil {
				logr.Printf(
					"failed to write method not allowed response:"+
						" request_id=%s method=%s path=%s error=%v",
					req.ID, req.Method(), req.Path(), err,
				)
			}

			return
		}

		ct := ContentType(r.Header.Get(contentTypeHeaderKey))
		if r.Method != GET && ct != "" && validateEndpointContentType(ct, endpoint.accepts) != nil {
			completionState.outcome = CompletionOutcomeUnsupportedContentType
			responsepkg.RenderErr(req, e.UnsupportedContentType(string(ct), joinContentTypes(endpoint.accepts)))
			req.Dur = time.Since(startedAt)
			written, err := writeRenderedResponse(w, req, timeout)
			responseSizeBytes = written
			if err != nil {
				logr.Printf(
					"failed to write unsupported content type response:"+
						" request_id=%s method=%s path=%s error=%v",
					req.ID, req.Method(), req.Path(), err,
				)
			}

			return
		}

		if limitErr := enforceEndpointRequestLimit(w, r, endpoint.LimitsSpec()); limitErr != nil {
			completionState.outcome = CompletionOutcomeRequestTooLarge
			responsepkg.RenderErr(req, limitErr)
			req.Dur = time.Since(startedAt)
			written, err := writeRenderedResponse(w, req, timeout)
			responseSizeBytes = written
			if err != nil {
				logr.Printf(
					"failed to write request too large response:"+
						" request_id=%s method=%s path=%s error=%v",
					req.ID, req.Method(), req.Path(), err,
				)
			}

			return
		}

		readDeadlineSet := setEndpointReadBodyDeadline(w, req, timeout)
		parsedReq, parseErr := requestpkg.NewReqWithError(r)
		if readDeadlineSet {
			clearEndpointReadBodyDeadline(w, req)
		}
		if endpointRequestTooLargeError(parseErr) {
			completionState.outcome = CompletionOutcomeRequestTooLarge
			responsepkg.RenderErr(req, e.RequestTooLarge(endpoint.LimitsSpec().MaxRequestBytes))
			req.Dur = time.Since(startedAt)
			written, err := writeRenderedResponse(w, req, timeout)
			responseSizeBytes = written
			if err != nil {
				logr.Printf(
					"failed to write request too large response:"+
						" request_id=%s method=%s path=%s error=%v",
					req.ID, req.Method(), req.Path(), err,
				)
			}

			return
		}
		if parsedReq == nil {
			completionState.outcome = CompletionOutcomeInvalidRequestBody
			responsepkg.RenderErr(req, e.InvalidRequestBody())
			req.Dur = time.Since(startedAt)
			written, err := writeRenderedResponse(w, req, timeout)
			responseSizeBytes = written
			if err != nil {
				logr.Printf(
					"failed to write invalid request body response:"+
						" request_id=%s method=%s path=%s error=%v",
					req.ID, req.Method(), req.Path(), err,
				)
			}

			return
		}
		req = parsedReq

		logr.Printf(
			"handling request: path=%s request=%s remote_addr=%s user_agent=%s",
			req.Path(), req.ID, req.RemoteAddr(),
			req.UserAgent(),
		)

		endpointHandler := endpoint.rawHandler
		if endpoint.idempotent {
			endpointHandler = func(req *Req) {
				handleIdempotently(
					req,
					endpoint.method,
					endpoint.pattern,
					endpoint.rawHandler,
					endpoint.resolver,
				)
			}
		}

		if accessErr := endpoint.accessError(req); accessErr != nil {
			completionState.outcome = CompletionOutcomeAccessDenied
			responsepkg.RenderErr(req, accessErr)
		} else {
			endpointHandler(req)
		}

		if req.Res == nil {
			if endpointTimedOut(req) {
				completionState.outcome = CompletionOutcomeTimedOut
				endpoint.handleTimeout(req)
			} else {
				logr.Printf(
					"endpoint returned without setting response:"+
						" request_id=%s method=%s path=%s",
					req.ID, req.Method(), req.Path(),
				)
				responsepkg.RenderErr(req, e.Unexpected())
			}
		}

		req.Dur = time.Since(startedAt)
		written, err := writeRenderedResponse(w, req, timeout)
		if written > 0 {
			responseSizeBytes = written
		}
		if err != nil {
			logr.Printf(
				"failed to write response:"+
					" request_id=%s method=%s path=%s error=%v",
				req.ID, req.Method(), req.Path(), err,
			)
		}
	}
}

func writeRenderedResponse(
	w http.ResponseWriter,
	req *Req,
	timeout EndpointTimeoutSpec,
) (int, error) {
	result, err := responsepkg.WriteResponse(w, req.Res, responsepkg.WriteOptions{
		RequestID:    req.ID,
		Caller:       req.RequestCaller(),
		Duration:     req.Dur,
		WriteTimeout: normalizeEndpointTimeoutSpec(timeout).Write,
	})
	if err == nil || result.Status != 0 {
		return result.BytesWritten, err
	}

	logr.Printf(
		"failed to encode response body:"+
			" request_id=%s method=%s path=%s error=%v",
		req.ID, req.Method(), req.Path(), err,
	)
	responsepkg.RenderErr(req, e.Unexpected())
	result, err = responsepkg.WriteResponse(w, req.Res, responsepkg.WriteOptions{
		RequestID:    req.ID,
		Caller:       req.RequestCaller(),
		Duration:     req.Dur,
		WriteTimeout: normalizeEndpointTimeoutSpec(timeout).Write,
	})
	return result.BytesWritten, err
}
