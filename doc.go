// Package httpapi documents the module-level package layout for
// github.com/inttegro/httpapi.
//
// Runtime behavior intentionally lives in subpackages. Use endpoint for endpoint
// definitions and groups, request for safe request parsing and authentication
// attachment, caller for provider-neutral caller labels, cost for provider-neutral
// usage observations, param for reusable request-parameter definitions,
// response for rendering and response writing, auth for provider-neutral
// auth/session contracts, server for default http.Server wiring, and openapi/*
// for spec transcription.
//
// The root package is intentionally doc-only so external consumers import the
// narrow package that owns the behavior they need.
//
// A typical service defines callers with caller.Define, configures request
// authentication and endpoint audit/idempotency adapters at startup, defines
// endpoint request parsers with param.JSON, defines caller-aware response shapes
// with response.Object or response.Project, records optional usage through
// request.Req cost helpers or cost.Record(ctx, usage), then wires handlers
// through endpoint.DefineEndpoint and serves them with endpoint.Mux and
// server.New.
package httpapi
