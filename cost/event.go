package cost

import (
	"time"
)

// RequestMetadata is the request-level metadata attached to an operation cost
// event.
//
// The fields are intentionally generic and audit-safe. They identify the
// completed operation enough for logging, metrics, and downstream pricing
// systems without carrying request bodies, authorization headers, raw tokens, or
// idempotency keys.
type RequestMetadata struct {
	// ID is the httpapi request identifier.
	ID string `json:"id,omitempty"`

	// ApplicationID is the authenticated application id, when available.
	ApplicationID string `json:"application_id,omitempty"`

	// SessionID is the authenticated session id, when available.
	SessionID string `json:"session_id,omitempty"`

	// Caller is the application-defined caller label attached to the request.
	Caller string `json:"caller,omitempty"`

	// Method is the inbound HTTP method.
	Method string `json:"method,omitempty"`

	// Path is the inbound request path without query parameters.
	Path string `json:"path,omitempty"`

	// ReceivedAt is when httpapi began handling the request.
	ReceivedAt time.Time `json:"received_at"`

	// CompletedAt is when endpoint handling completed.
	CompletedAt time.Time `json:"completed_at"`

	// Duration is the elapsed endpoint runtime duration.
	Duration time.Duration `json:"duration,omitempty"`

	// Status is the final HTTP response status code, when one was selected.
	Status int `json:"status,omitempty"`

	// Outcome identifies the endpoint runtime path that completed the request.
	Outcome string `json:"outcome,omitempty"`

	// ResponseSizeBytes is the number of response bytes written when known.
	ResponseSizeBytes int `json:"response_size_bytes,omitempty"`
}

// EndpointMetadata is the endpoint-level metadata attached to an operation
// cost event.
type EndpointMetadata struct {
	// Method is the HTTP method accepted by the endpoint.
	Method string `json:"method,omitempty"`

	// Pattern is the endpoint path pattern.
	Pattern string `json:"pattern,omitempty"`

	// OperationID is the provider-neutral operation id from endpoint operation
	// metadata.
	OperationID string `json:"operation_id,omitempty"`

	// OperationKind is the endpoint's explicit read/write classification.
	OperationKind string `json:"operation_kind,omitempty"`

	// Summary is the human-readable operation summary from endpoint metadata.
	Summary string `json:"summary,omitempty"`

	// Internal reports whether the endpoint is internal-only.
	Internal bool `json:"internal,omitempty"`

	// Priority is the endpoint operational priority.
	Priority string `json:"priority,omitempty"`

	// Idempotent reports whether the endpoint enforces idempotency.
	Idempotent bool `json:"idempotent,omitempty"`
}

// OperationEvent describes the usage units observed while handling one request
// or operation.
//
// OperationEvent is the boundary between httpapi runtime observation and
// service-owned cost policy. It contains enough metadata for downstream systems
// to estimate cost per operation, but it does not contain USD prices, billing
// account configuration, discounts, invoice data, or reconciliation state.
type OperationEvent struct {
	// Operation identifies the unit of work that produced this usage event.
	Operation Operation `json:"operation"`

	// Request describes the completed request.
	Request RequestMetadata `json:"request"`

	// Endpoint describes the endpoint that handled the request.
	Endpoint EndpointMetadata `json:"endpoint"`

	// Usage contains provider-neutral usage units observed for the operation.
	Usage []UsageUnit `json:"usage,omitempty"`

	// RecordedAt is when this event snapshot was produced.
	RecordedAt time.Time `json:"recorded_at"`
}

// Empty reports whether event contains no usage units.
func (event OperationEvent) Empty() bool {
	return len(event.Usage) == 0
}

// Clone returns a copy of event with cloned usage labels.
func (event OperationEvent) Clone() OperationEvent {
	event.Usage = cloneUsageUnits(event.Usage)
	return event
}
