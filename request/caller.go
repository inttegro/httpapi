package request

import (
	"context"

	callerpkg "github.com/inttegro/httpapi/caller"
)

type callerContextKey struct{}
type responseCallerContextKey struct{}

type responseProjectionExecutor struct {
	ctx       context.Context
	req       *Req
	policy    callerpkg.ProjectionPolicy
	requested Caller
	granted   Caller
	projected Caller
	err       error
}

// Caller is the provider-neutral request caller label used by endpoint,
// parameter, and response-attribute availability rules.
type Caller = callerpkg.Caller

// ContextWithCaller returns a child context carrying caller.
//
// Trusted middleware should attach the caller before endpoint handling begins.
// NewReq reads the value back from the request context and stores it on Req so
// endpoint access checks and parameter parsing use this caller. Response
// projection uses it by default, but trusted middleware can request a narrower
// caller with ContextWithResponseProjection.
func ContextWithCaller(ctx context.Context, caller Caller) context.Context {
	if !caller.Defined() {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// CallerFromContext returns a caller previously installed by ContextWithCaller.
func CallerFromContext(ctx context.Context) Caller {
	if ctx == nil {
		return Caller{}
	}
	caller, _ := ctx.Value(callerContextKey{}).(Caller)
	return caller
}

// ContextWithResponseProjection returns a child context carrying the response
// projection allowed by policy for the request caller already in ctx.
//
// Identity and declared transitive descent are allowed. Elevation and lateral
// projection return caller.ErrProjectionElevation without modifying ctx.
func ContextWithResponseProjection(
	ctx context.Context,
	policy callerpkg.ProjectionPolicy,
	requested Caller,
) (context.Context, error) {
	executor := responseProjectionExecutor{
		ctx:       ctx,
		policy:    policy,
		requested: requested,
	}
	executor.loadGrantedCaller()
	executor.resolve()
	executor.attachToContext()
	return executor.ctx, executor.err
}

func responseCallerFromContext(ctx context.Context) Caller {
	if ctx == nil {
		return Caller{}
	}
	caller, _ := ctx.Value(responseCallerContextKey{}).(Caller)
	return caller
}

// AttachCaller attaches caller to the parsed request.
func (r *Req) AttachCaller(caller Caller) {
	if r == nil || !caller.Defined() {
		return
	}
	r.Caller = caller
}

// ProjectResponseAs applies the requested response projection when policy
// permits it for RequestCaller. The request caller remains unchanged.
func (r *Req) ProjectResponseAs(policy callerpkg.ProjectionPolicy, requested Caller) error {
	executor := responseProjectionExecutor{
		req:       r,
		policy:    policy,
		requested: requested,
	}
	executor.loadGrantedCaller()
	executor.resolve()
	executor.attachToRequest()
	return executor.err
}

func (executor *responseProjectionExecutor) loadGrantedCaller() {
	if executor.err != nil {
		return
	}
	if executor.req != nil {
		executor.granted = executor.req.RequestCaller()
		return
	}
	executor.granted = CallerFromContext(executor.ctx)
}

func (executor *responseProjectionExecutor) resolve() {
	if executor.err != nil {
		return
	}
	executor.projected, executor.err = executor.policy.Resolve(
		executor.granted,
		executor.requested,
	)
}

func (executor *responseProjectionExecutor) attachToContext() {
	if executor.err != nil {
		return
	}
	executor.ctx = context.WithValue(
		executor.ctx,
		responseCallerContextKey{},
		executor.projected,
	)
}

func (executor *responseProjectionExecutor) attachToRequest() {
	if executor.err != nil {
		return
	}
	if executor.req == nil {
		executor.err = callerpkg.ErrProjectionCallerUndefined
		return
	}
	executor.req.responseCaller = executor.projected
}

// RequestCaller returns the application-defined caller attached to the request.
//
// RequestCaller is the small interface shared with param.WithRequestCaller.
// Prefer it over reading Req.Caller directly when another package only needs
// request authority.
func (r *Req) RequestCaller() Caller {
	if r == nil {
		return Caller{}
	}
	return r.Caller
}

// ResponseCaller returns the caller used to project response attributes. It
// defaults to RequestCaller when no narrower projection was attached.
func (r *Req) ResponseCaller() Caller {
	if r == nil {
		return Caller{}
	}
	if r.responseCaller.Defined() {
		return r.responseCaller
	}
	return r.RequestCaller()
}
