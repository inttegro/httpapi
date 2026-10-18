// Package request owns safe request parsing, request audit serialization, and
// request-scoped authentication state for httpapi endpoint runtimes.
//
// Req is deliberately not an endpoint contract. It buffers and restores the
// incoming body, exposes normalized accessors, attaches sessions, and produces
// redacted audit JSON while endpoint.Endpoint decides what the request must
// satisfy.
//
// Trusted middleware can attach an application-defined caller with
// ContextWithCaller and an already authenticated session with ContextWithSession.
// NewReq also knows how to call the configured Authenticator when credentials
// are present. Response rendering reads ResponseContext from Req so handlers can
// render context-aware response bodies without passing authority or property
// selection by hand.
//
// NewReq creates a request operation recorder and attaches it to the underlying
// context before authentication runs. Handlers and helper packages can attach
// provider-neutral usage units with the Req cost helpers or cost.Record(ctx,
// usage). Endpoint completion observers receive those units as a
// cost.OperationEvent; request audit serialization does not price or reconcile
// them.
//
// Req audit serialization is intentionally conservative. Authorization headers,
// idempotency keys, request bodies, and response bodies are redacted or
// summarized so services can persist request audit records without leaking
// common credential or payload material.
package request
