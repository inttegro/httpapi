// Package openapi31 transcribes httpapi routes into public OpenAPI 3.1
// documents.
//
// Internal endpoints are intentionally omitted. Endpoint operation metadata
// supplies operation IDs and summaries; access metadata supplies authorization
// requirements and priority.
//
// Response schemas currently aggregate attributes across caller-specific
// views; attribute availability is not encoded in the OpenAPI document. The
// generated document therefore describes possible payload shapes and must not
// be treated as an authorization policy. Runtime response projection remains
// responsible for enforcing caller visibility.
package openapi31
