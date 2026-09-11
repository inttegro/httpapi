// Package response owns response objects, render helpers, caller-aware response
// shapes, body encoding, and HTTP response writing for httpapi endpoint
// runtimes.
//
// Handlers can either construct a Res with JSON, Text, HTML, Bytes, Empty,
// NoContent, Redirect, or Stream and pass it to Render, or call convenience
// helpers such as RenderJSON, RenderErr, RenderNoContent, RenderRedirect, and
// RenderStream. Endpoint runtimes call WriteResponse after the handler returns
// so standard headers, timing, request IDs, streaming, raw bytes, and write
// deadlines are applied consistently. CORS belongs at the server middleware
// boundary, not in response writing.
//
// JSON object responses can also declare a response shape. A response shape is
// a projection contract: handlers still return normal Go values, while the
// shape explicitly lists which JSON attributes are emitted, their value types,
// and whether particular attributes are visible only to selected caller.Caller
// values. When a shaped body is rendered through a request target, response
// derives the caller from the target and omits attributes unavailable to that
// caller.
//
// The same caller model is shared by endpoints, request parameters, and response
// attributes. This keeps authorization-sensitive request and response contracts
// aligned without introducing response-specific service labels.
//
// The most direct response shape is Object:
//
//	type taskView struct {
//		ID           string
//		Status       string
//		InternalNote string
//	}
//
//	var taskResponse = response.Object[taskView](
//		response.Required("id", response.String(), func(task taskView) string { return task.ID }),
//		response.Required("status", response.String(), func(task taskView) string { return task.Status }),
//		response.Optional("internal_note", response.String(), func(task taskView) (string, bool) {
//			return task.InternalNote, task.InternalNote != ""
//		}).AvailableTo(Worker),
//	)
//
// Use Project when the handler has a domain value that should be prepared once
// before attributes are extracted. This keeps nil checks, defaults, and
// redaction in a small view adapter while the object shape stays explicit:
//
//	type taskView struct{ task Task }
//
//	func taskValue(task *Task) taskView {
//		if task == nil {
//			return taskView{}
//		}
//		return taskView{task: *task}
//	}
//
//	func (task taskView) ID() string     { return task.task.ID }
//	func (task taskView) Status() string { return task.task.Status }
//
//	var taskResponse = response.Project(
//		response.Object[taskView](
//			response.Required("id", response.String(), taskView.ID),
//			response.Required("status", response.String(), taskView.Status),
//		),
//		taskValue,
//	)
//
// Handlers should render shaped responses with shape.Body(value). RenderJSON and
// WriteResponse project the body for the caller attached to the request target.
//
// Use Envelope when the response body is only a top-level object that names one
// or more already-shaped values. This removes the need for endpoint-local
// wrapper structs such as type Response struct { Task *taskView }:
//
//	var taskEnvelope = response.Envelope(
//		response.OptionalField("task", taskResponse),
//		response.OptionalField("error", response.ErrorObjectShape),
//	)
//
//	response.RenderJSON(r, http.StatusOK, taskEnvelope.Body(
//		response.Field("task", task),
//	))
//
// Envelope is intentionally small: it wraps values under names, applies caller
// availability, and exposes the same ShapeSpec metadata as Object. Each accepted
// field must have a distinct Go value type; define small named types and use
// Project when two JSON fields would otherwise share an underlying scalar type.
// An envelope must define at least one accepted attribute, and projecting it
// must emit at least one caller-visible, real field. Optional nil fields are
// omitted and do not satisfy the envelope's non-empty requirement. RequiredField
// describes schema and per-field requiredness: a missing or nil required field
// panics, but RequiredField is not needed merely to make the envelope non-empty.
// Keep using Object or Project when individual attributes need getters,
// defaults, redaction, or domain-to-response preparation. Envelope also
// implements Shape, so it can describe nested object values that do not need
// their own Go view struct.
//
// Use Meta in an envelope when the response can include the standard
// response_meta object. Build values with the same compact style as headers:
//
//	var taskEnvelope = response.Envelope(
//		response.OptionalField("task", taskResponse),
//		response.Meta(),
//	)
//
//	response.RenderJSON(r, http.StatusOK, taskEnvelope.Body(
//		response.Field("task", task),
//		response.WithMeta(response.Meta().
//			Set("processing_ms", 84).
//			Debug("provider_attempts", 1)),
//	))
//
// response_meta is curated body metadata. HTTP status and headers remain native
// HTTP response fields; do not place raw requests, authorization material,
// idempotency keys, provider payloads, stack traces, or secrets in response_meta.
package response
