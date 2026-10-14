package response

import callerpkg "github.com/zebodotdev/httpapi/caller"

// ResponseCallerTarget is a response target that can expose the caller used to
// project the response being rendered.
//
// request.Req satisfies this interface. Render uses it to project shaped bodies
// without asking endpoint handlers to pass caller information manually.
type ResponseCallerTarget interface {
	Target

	// ResponseCaller returns the application-defined response projection caller.
	ResponseCaller() callerpkg.Caller
}

// CallerTarget is the legacy response target interface used before request
// authority and response projection were separated. Custom targets that only
// implement CallerTarget remain supported.
type CallerTarget interface {
	Target

	RequestCaller() callerpkg.Caller
}

// CallerProjector is a response body that can project itself for a caller.
//
// ShapedBody implements CallerProjector. Custom body types may implement it
// when they need caller-aware projection but do not fit ObjectShape.
type CallerProjector interface {
	// ProjectResponse returns the body visible to caller.
	ProjectResponse(caller callerpkg.Caller) any
}

func responseForTarget(target Target, res *Res) *Res {
	if res == nil {
		return nil
	}

	body, ok := projectResponseBody(res.Body, callerFromTarget(target))
	if !ok {
		return res
	}

	projected := *res
	projected.Header = cloneHeader(res.Header)
	projected.Body = body
	return &projected
}

func callerFromTarget(target Target) callerpkg.Caller {
	responseCallerTarget, ok := target.(ResponseCallerTarget)
	if ok {
		return responseCallerTarget.ResponseCaller()
	}
	callerTarget, ok := target.(CallerTarget)
	if !ok {
		return callerpkg.Caller{}
	}
	return callerTarget.RequestCaller()
}

func projectResponseBody(body any, caller callerpkg.Caller) (any, bool) {
	projector, ok := body.(CallerProjector)
	if !ok {
		return body, false
	}
	return projector.ProjectResponse(caller), true
}
