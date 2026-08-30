// Package endpoint defines httpapi's typed HTTP endpoint runtime and
// provider-neutral operation and route metadata.
//
// The package is responsible for the HTTP concerns that belong around an
// application handler: method and content-type checks, request-size limits,
// authentication and caller availability, idempotency replay, handler timeouts,
// response writing, completion hooks, and route metadata for generated API
// documents. Domain validation and business work should stay in application
// packages; endpoint responders should parse the request, call that domain
// code, and return one response.
//
// # Defining Endpoints
//
// DefineEndpoint is the general entry point for new endpoints. It collects the
// endpoint's method, path, responder, access policy, idempotency policy,
// priority, timeout budget, request limit, operation metadata, route metadata,
// payload contracts, and response contracts in one EndpointSpec. Respond is the preferred
// return-style handler; Handler is kept for compatibility with existing
// render-in-place integrations. Keeping endpoint requirements on EndpointSpec
// lets request.Req remain a safe parse of an incoming HTTP request rather than a
// route-specific contract object.
//
// DefineJSONEndpoint is the typed JSON convenience entry point. It binds a
// param.Request parser to the endpoint, uses the same parser as the request-body
// contract for documentation, renders param errors with the standard httpapi
// error envelope, and invokes a typed RequestResponder only after parsing
// succeeds. Use HandlerWithRequestResponder when an existing EndpointSpec should
// receive the same typed parsing behavior. HandlerWithRequest remains available
// for compatibility.
//
// Endpoint requirements belong on EndpointSpec. A request.Req only represents a
// safely parsed incoming HTTP request with caller and session state attached; it
// should not know which route will eventually handle it. The endpoint runtime
// combines the Req with the EndpointSpec and enforces method, content type,
// authorization, caller availability, request size limits, idempotency, and
// timeout policy before or around the handler.
//
// # Runtime Outcomes
//
// The runtime may complete a request before the application handler runs. For
// example, unsupported methods, unsupported content types, oversized requests,
// unreadable bodies, missing authorization, denied callers, idempotency
// conflicts, and handler timeouts are all handled by the endpoint wrapper.
// Handler panics are recorded for audit and completion observers before being
// re-panicked, preserving net/http panic behavior.
//
// # Completion and Audit Hooks
//
// ConfigureAuditSink installs request-audit persistence that receives the
// completed Req. ConfigureCompletionSink installs a service-owned observer for
// logging, metrics, notifications, or secondary audit streams. Completion
// events include endpoint metadata, request metadata, status, duration,
// response size, a coarse CompletionOutcome, structured httpapi error metadata
// when available, panic metadata when a handler panics, and a provider-neutral
// cost.OperationEvent containing operation correlation plus any usage units
// recorded on the request. Sinks are intentionally package-neutral; services
// decide how to redact, persist, price, reconcile, or route completion events.
//
// # Route Metadata and Transcription
//
// Endpoint metadata is provider-neutral. OperationSpec describes operation ID,
// summary, explicit read/write kind, and accounting metadata. Operation.Kind is
// never inferred from the HTTP method or route and is never inherited from an
// endpoint group. Operation.ID is the shared identity used by OpenAPI,
// generated docs, completion events, and cost accounting. RouteSpec is
// routing/backend metadata only: backend addresses, path forwarding behavior,
// and backend timeout intent without embedding a cloud provider's document
// format. RequestContract and ResponseContract describe payloads by reusing
// param and response shape metadata. Target transcribers under openapi/*
// translate that metadata into concrete OpenAPI or gateway documents.
//
// # Groups and Muxes
//
// EndpointGroup applies shared defaults to several endpoints. Group caller
// availability narrows endpoint availability; it cannot widen an endpoint that
// was already restricted. Empty caller availability means available to all.
//
// Mux is httpapi's preferred serving surface. It implements http.Handler, wraps
// the standard library ServeMux, mounts endpoint groups with inherited metadata,
// rejects duplicate method/path pairs before registration, and keeps a mounted
// endpoint snapshot for docs and operational tooling. Applications that already
// own a *http.ServeMux can still pass it with WithServeMux.
//
// NewEndpoint, NewIdempotentEndpoint, and
// NewIdempotentEndpointWithScopeResolver exist for compatibility with older
// integrations. Prefer DefineEndpoint for new code because a named EndpointSpec
// remains readable as endpoint requirements grow.
package endpoint
