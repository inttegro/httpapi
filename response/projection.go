package response

// Projector is a response body that can project itself in a rendering context.
//
// ShapedBody implements Projector. Custom body types may implement it when they
// need context-aware projection but do not fit ObjectShape.
type Projector interface {
	Project(Context) any
}

func responseForTarget(target Target, res *Res) *Res {
	if res == nil {
		return nil
	}

	body, ok := projectResponseBody(res.Body, contextFromTarget(target))
	if !ok {
		return res
	}

	projected := *res
	projected.Header = cloneHeader(res.Header)
	projected.Body = body
	return &projected
}

func contextFromTarget(target Target) Context {
	contextual, ok := target.(ContextTarget)
	if !ok {
		return Context{}
	}
	return contextual.ResponseContext()
}

func projectResponseBody(body any, context Context) (any, bool) {
	projector, ok := body.(Projector)
	if !ok {
		return body, false
	}
	return projector.Project(context), true
}
