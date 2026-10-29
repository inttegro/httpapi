package endpoint

import (
	"context"
	"errors"
	"net/http"
	"time"

	e "github.com/inttegro/httpapi/erreur"
	responsepkg "github.com/inttegro/httpapi/response"
)

// EndpointTimeoutSpec declares runtime timeout budgets for one endpoint.
//
// These budgets are enforced by the in-process HTTP wrapper. They are separate
// from RouteBackend.Timeout, which only describes generated gateway/backend
// deadlines for transcribed route specs.
type EndpointTimeoutSpec = TimeoutSpec

// EndpointTimeoutHandler renders the response for an endpoint whose handler
// context exceeded its configured timeout before a response was produced.
type EndpointTimeoutHandler func(*Req)

type endpointTimeoutPolicy struct {
	timeout EndpointTimeoutSpec
	handler EndpointTimeoutHandler

	readBodyInherited       bool
	handlerInherited        bool
	writeInherited          bool
	timeoutHandlerInherited bool
}

type endpointDeadlineSetter func(*http.ResponseController, time.Time) error

// WithTimeoutSpec applies runtime timeout budgets to an endpoint.
//
// It replaces all timeout fields on the endpoint. Use EndpointGroup defaults
// when you want unset endpoint fields to inherit group-level values.
func WithTimeoutSpec(spec EndpointTimeoutSpec) EndpointOption {
	spec = normalizeEndpointTimeoutSpec(spec)
	return func(e *Endpoint) {
		e.timeout.timeout = spec
		e.timeout.readBodyInherited = false
		e.timeout.handlerInherited = false
		e.timeout.writeInherited = false
	}
}

// WithTimeoutHandler sets the endpoint-specific timeout response handler.
//
// If the handler does not set a response, httpapi falls back to
// DefaultEndpointTimeoutHandler.
func WithTimeoutHandler(handler EndpointTimeoutHandler) EndpointOption {
	return func(e *Endpoint) {
		e.timeout.handler = handler
		e.timeout.timeoutHandlerInherited = false
	}
}

// ConfigureTimeoutSpec sets default runtime timeout budgets for endpoints in
// the group.
//
// Endpoint-level timeout fields override group defaults field by field.
func (eg *EndpointGroup) ConfigureTimeoutSpec(spec EndpointTimeoutSpec) {
	eg.Timeout = normalizeEndpointTimeoutSpec(spec)
	for i := range eg.Endpoints {
		eg.Endpoints[i].mutableTimeoutPolicy().inheritDefaults(eg.Timeout)
		eg.Endpoints[i] = eg.Endpoints[i].withRebuiltHandler()
	}
}

// ConfigureTimeoutHandler sets the default timeout response handler for
// endpoints in the group.
//
// Endpoint-level timeout handlers override the group handler.
func (eg *EndpointGroup) ConfigureTimeoutHandler(handler EndpointTimeoutHandler) {
	eg.TimeoutHandler = handler
	for i := range eg.Endpoints {
		eg.Endpoints[i].mutableTimeoutPolicy().inheritHandler(handler)
		eg.Endpoints[i] = eg.Endpoints[i].withRebuiltHandler()
	}
}

// TimeoutSpec returns the group's normalized default runtime timeout budgets.
func (eg EndpointGroup) TimeoutSpec() EndpointTimeoutSpec {
	return normalizeEndpointTimeoutSpec(eg.Timeout)
}

// TimeoutSpec returns the endpoint's normalized runtime timeout budgets.
func (e Endpoint) TimeoutSpec() EndpointTimeoutSpec {
	return e.timeoutSpec()
}

func (e Endpoint) timeoutSpec() EndpointTimeoutSpec {
	return normalizeEndpointTimeoutSpec(e.timeout.timeout)
}

func (e *Endpoint) mutableTimeoutPolicy() *endpointTimeoutPolicy {
	return &e.timeout
}

func (p *endpointTimeoutPolicy) inheritDefaults(defaults EndpointTimeoutSpec) {
	defaults = normalizeEndpointTimeoutSpec(defaults)
	p.timeout = normalizeEndpointTimeoutSpec(p.timeout)

	if p.timeout.ReadBody == 0 || p.readBodyInherited {
		p.timeout.ReadBody = defaults.ReadBody
		p.readBodyInherited = defaults.ReadBody != 0
	}
	if p.timeout.Handler == 0 || p.handlerInherited {
		p.timeout.Handler = defaults.Handler
		p.handlerInherited = defaults.Handler != 0
	}
	if p.timeout.Write == 0 || p.writeInherited {
		p.timeout.Write = defaults.Write
		p.writeInherited = defaults.Write != 0
	}
}

func (p *endpointTimeoutPolicy) inheritHandler(handler EndpointTimeoutHandler) {
	if p.handler == nil || p.timeoutHandlerInherited {
		p.handler = handler
		p.timeoutHandlerInherited = handler != nil
	}
}

// DefaultEndpointTimeoutHandler renders the default timeout response.
func DefaultEndpointTimeoutHandler(req *Req) {
	responsepkg.RenderErr(req, e.RequestTimeout())
}

func (e Endpoint) handleTimeout(req *Req) {
	if e.timeout.handler != nil {
		e.timeout.handler(req)
		if req != nil && req.Res != nil {
			return
		}
	}
	DefaultEndpointTimeoutHandler(req)
}

func normalizeEndpointTimeoutSpec(spec EndpointTimeoutSpec) EndpointTimeoutSpec {
	return NormalizeTimeoutSpec(spec)
}

func requestWithEndpointHandlerTimeout(
	req *http.Request,
	timeout EndpointTimeoutSpec,
	startedAt time.Time,
) (*http.Request, context.CancelFunc) {
	timeout = normalizeEndpointTimeoutSpec(timeout)
	if req == nil || timeout.Handler == 0 {
		return req, nil
	}

	ctx, cancel := context.WithDeadline(
		req.Context(),
		startedAt.Add(timeout.Handler),
	)
	return req.WithContext(ctx), cancel
}

func endpointTimedOut(req *Req) bool {
	if req == nil {
		return false
	}

	return errors.Is(req.Context().Err(), context.DeadlineExceeded)
}

func setEndpointReadBodyDeadline(
	w http.ResponseWriter,
	req *Req,
	timeout EndpointTimeoutSpec,
) bool {
	timeout = normalizeEndpointTimeoutSpec(timeout)
	if timeout.ReadBody == 0 {
		return false
	}

	return applyEndpointDeadline(
		w,
		endpointDeadlineLogContext{
			RequestID: requestIDForDeadlineLog(req),
			Method:    methodForDeadlineLog(req),
			Path:      pathForDeadlineLog(req),
		},
		time.Now().Add(timeout.ReadBody),
		"read_body",
		(*http.ResponseController).SetReadDeadline,
	)
}

func clearEndpointReadBodyDeadline(w http.ResponseWriter, req *Req) {
	applyEndpointDeadline(
		w,
		endpointDeadlineLogContext{
			RequestID: requestIDForDeadlineLog(req),
			Method:    methodForDeadlineLog(req),
			Path:      pathForDeadlineLog(req),
		},
		time.Time{},
		"read_body",
		(*http.ResponseController).SetReadDeadline,
	)
}

func setEndpointWriteDeadline(
	w http.ResponseWriter,
	requestID string,
	timeout EndpointTimeoutSpec,
) bool {
	timeout = normalizeEndpointTimeoutSpec(timeout)
	if timeout.Write == 0 {
		return false
	}

	return applyEndpointDeadline(
		w,
		endpointDeadlineLogContext{RequestID: requestID},
		time.Now().Add(timeout.Write),
		"write",
		(*http.ResponseController).SetWriteDeadline,
	)
}

func clearEndpointWriteDeadline(w http.ResponseWriter, requestID string) {
	applyEndpointDeadline(
		w,
		endpointDeadlineLogContext{RequestID: requestID},
		time.Time{},
		"write",
		(*http.ResponseController).SetWriteDeadline,
	)
}

type endpointDeadlineLogContext struct {
	RequestID string
	Method    string
	Path      string
}

func applyEndpointDeadline(
	w http.ResponseWriter,
	logContext endpointDeadlineLogContext,
	deadline time.Time,
	phase string,
	set endpointDeadlineSetter,
) bool {
	if w == nil {
		return false
	}

	if err := set(http.NewResponseController(w), deadline); err != nil {
		if errors.Is(err, http.ErrNotSupported) {
			return false
		}

		logr.Printf(
			"failed to set endpoint %s deadline:"+
				" request_id=%s method=%s path=%s deadline=%v error=%v",
			phase,
			logContext.RequestID,
			logContext.Method,
			logContext.Path,
			deadline,
			err,
		)
		return false
	}

	return true
}

func requestIDForDeadlineLog(req *Req) string {
	if req == nil {
		return ""
	}

	return req.ID
}

func methodForDeadlineLog(req *Req) string {
	if req == nil || req.Req == nil {
		return ""
	}

	return req.Req.Method
}

func pathForDeadlineLog(req *Req) string {
	if req == nil || req.Req == nil || req.Req.URL == nil {
		return ""
	}

	return req.Req.URL.Path
}
