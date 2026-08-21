package cost

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// IdType is the operation ID prefix used by NewOperationID.
	IdType = "op"
)

// Operation identifies one unit of work whose usage can be estimated.
//
// Operation metadata is intentionally generic. A request, job, workflow,
// activity, provider call, or repository call can all have an operation id and
// can all point back to the same root operation for accounting rollups.
type Operation struct {
	// ID uniquely identifies this operation event.
	ID string `json:"operation_id,omitempty"`

	// RootID identifies the top-level operation that caused this operation.
	RootID string `json:"root_operation_id,omitempty"`

	// ParentID identifies the immediate operation that caused this operation.
	ParentID string `json:"parent_operation_id,omitempty"`

	// TraceID carries a provider-neutral distributed trace id when available.
	TraceID string `json:"trace_id,omitempty"`

	// CausationRequestID identifies the request that caused this operation when
	// the operation was triggered directly or indirectly by an HTTP request.
	CausationRequestID string `json:"causation_request_id,omitempty"`

	// Name is an application-defined operation name such as an endpoint route
	// operation id, job name, workflow name, activity name, or provider call.
	Name string `json:"name,omitempty"`

	// Kind is a coarse application-defined category such as "http_request",
	// "job", "workflow", "activity", "repository", or "provider".
	Kind string `json:"kind,omitempty"`

	// StartedAt is when the operation started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the operation completed.
	CompletedAt time.Time `json:"completed_at"`
}

// NewOperationID returns a new provider-neutral operation identifier.
func NewOperationID() string {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err == nil {
		return IdType + "_" + hex.EncodeToString(buf)
	}

	return fmt.Sprintf("%s_%d", IdType, time.Now().UnixNano())
}

func normalizeOperation(operation Operation) Operation {
	operation.ID = strings.TrimSpace(operation.ID)
	operation.RootID = strings.TrimSpace(operation.RootID)
	operation.ParentID = strings.TrimSpace(operation.ParentID)
	operation.TraceID = strings.TrimSpace(operation.TraceID)
	operation.CausationRequestID = strings.TrimSpace(operation.CausationRequestID)
	operation.Name = strings.TrimSpace(operation.Name)
	operation.Kind = strings.TrimSpace(operation.Kind)

	if operation.ID == "" {
		operation.ID = NewOperationID()
	}
	if operation.RootID == "" {
		if operation.ParentID != "" {
			operation.RootID = operation.ParentID
		} else {
			operation.RootID = operation.ID
		}
	}
	if operation.StartedAt.IsZero() {
		operation.StartedAt = time.Now()
	}

	return operation
}

func childOperation(parent Operation, operation Operation) Operation {
	rootProvided := strings.TrimSpace(operation.RootID) != ""
	parentProvided := strings.TrimSpace(operation.ParentID) != ""
	traceProvided := strings.TrimSpace(operation.TraceID) != ""
	causationProvided := strings.TrimSpace(operation.CausationRequestID) != ""

	operation = normalizeOperation(operation)
	parent = normalizeOperation(parent)

	if !parentProvided {
		operation.ParentID = parent.ID
	}
	if !rootProvided {
		operation.RootID = parent.RootID
	}
	if !traceProvided {
		operation.TraceID = parent.TraceID
	}
	if !causationProvided {
		operation.CausationRequestID = parent.CausationRequestID
	}

	return operation
}
