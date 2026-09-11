package response

import (
	"net/http"
	"strings"
	"testing"
)

func TestResponseMetaProjectsThroughEnvelope(t *testing.T) {
	shape := Envelope(
		RequiredField("record", recordResponseShape()),
		Meta(),
	)

	got := shape.ProjectForCaller(Fields(
		Field("record", sampleRecordResponse()),
		WithMeta(Meta().
			RequestID("req_123").
			Set("processing_ms", 84).
			Debug("routing", "primary").
			Debug("provider_attempts", 1)),
	), responseWorkerCaller)

	rawMeta, ok := got[ResponseMetaName].(map[string]any)
	if !ok {
		t.Fatalf("response_meta = %T, want map[string]any", got[ResponseMetaName])
	}
	if rawMeta["request_id"] != "req_123" {
		t.Fatalf("request_id = %#v, want req_123", rawMeta["request_id"])
	}
	if rawMeta["processing_ms"] != 84 {
		t.Fatalf("processing_ms = %#v, want 84", rawMeta["processing_ms"])
	}
	debug, ok := rawMeta["debug"].(map[string]any)
	if !ok {
		t.Fatalf("debug = %T, want map[string]any", rawMeta["debug"])
	}
	if debug["routing"] != "primary" || debug["provider_attempts"] != 1 {
		t.Fatalf("debug = %#v", debug)
	}

	spec := Describe(shape)
	meta := findAttributeSpec(t, spec, ResponseMetaName)
	if meta.Required {
		t.Fatal("response_meta was marked required")
	}
	if meta.Shape.Type != TypeObject {
		t.Fatalf("response_meta type = %q, want object", meta.Shape.Type)
	}
	if meta.Shape.MapValue == nil || meta.Shape.MapValue.Type != TypeAny {
		t.Fatalf("response_meta map value = %#v, want any", meta.Shape.MapValue)
	}
}

func TestResponseMetaOmitsEmptyValues(t *testing.T) {
	shape := Envelope(
		OptionalField("status", String()),
		Meta(),
	)

	got := shape.ProjectForCaller(Fields(
		Field("status", "ok"),
		WithMeta(Meta()),
	), responseWorkerCaller)

	if _, ok := got[ResponseMetaName]; ok {
		t.Fatalf("empty response_meta was emitted: %#v", got)
	}
	if got["status"] != "ok" {
		t.Fatalf("status = %#v, want ok", got["status"])
	}
}

func TestResponseMetaBuilderReturnsValueSnapshot(t *testing.T) {
	meta := Meta().
		Set("processing_ms", 84).
		Debug("routing", "primary")

	values := meta.Values()
	values["processing_ms"] = 1
	values["debug"].(map[string]any)["routing"] = "mutated"

	got := meta.Values()
	if got["processing_ms"] != 84 {
		t.Fatalf("processing_ms leaked mutation: %#v", got["processing_ms"])
	}
	debug := got["debug"].(map[string]any)
	if debug["routing"] != "primary" {
		t.Fatalf("debug leaked mutation: %#v", debug)
	}
}

func TestResponseMetaDeletesNilValues(t *testing.T) {
	meta := Meta().
		Set("processing_ms", 84).
		Set("processing_ms", nil).
		Debug("routing", "primary").
		Debug("routing", nil)

	if !meta.Empty() {
		t.Fatalf("meta values = %#v, want empty", meta.Values())
	}
}

func TestResponseMetaPanicsForUnsafeKeysAndValues(t *testing.T) {
	tests := []struct {
		name string
		run  func()
	}{
		{
			name: "empty key",
			run: func() {
				Meta().Set(" ", "value")
			},
		},
		{
			name: "authorization key",
			run: func() {
				Meta().Set("authorization", "Bearer secret")
			},
		},
		{
			name: "request body key",
			run: func() {
				Meta().Debug("request_body", map[string]any{"card": "nope"})
			},
		},
		{
			name: "http header value",
			run: func() {
				Meta().Set("headers", http.Header{"Authorization": {"Bearer secret"}})
			},
		},
		{
			name: "http header pointer",
			run: func() {
				headers := http.Header{"Authorization": {"Bearer secret"}}
				Meta().Set("headers", &headers)
			},
		},
		{
			name: "nested unsafe key",
			run: func() {
				Meta().Set("stats", map[string]any{"authorization": "Bearer secret"})
			},
		},
		{
			name: "nested unsafe value",
			run: func() {
				Meta().Set("stats", map[string]any{"headers": http.Header{"Authorization": {"Bearer secret"}}})
			},
		},
		{
			name: "slice unsafe value",
			run: func() {
				Meta().Set("events", []any{http.Header{"Authorization": {"Bearer secret"}}})
			},
		},
		{
			name: "typed slice unsafe value",
			run: func() {
				Meta().Set("events", []map[string]any{{"authorization": "Bearer secret"}})
			},
		},
		{
			name: "non string map key",
			run: func() {
				Meta().Set("stats", map[int]any{1: "count"})
			},
		},
		{
			name: "raw bytes",
			run: func() {
				Meta().Set("payload", []byte("raw"))
			},
		},
		{
			name: "request value",
			run: func() {
				Meta().Set("request", httptestRequest(t))
			},
		},
		{
			name: "stream value",
			run: func() {
				Meta().Set("stream", strings.NewReader("raw"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Fatal("expected panic")
				}
			}()
			tt.run()
		})
	}
}

func TestResponseMetaAvailableToRestrictsEnvelopeProjection(t *testing.T) {
	shape := Envelope(
		OptionalField("status", String()),
		Meta().AvailableTo(responseWorkerCaller),
	)
	fields := Fields(
		Field("status", "ok"),
		WithMeta(Meta().Set("processing_ms", 84)),
	)

	public := shape.ProjectForCaller(fields, responsePublicCaller)
	if _, ok := public[ResponseMetaName]; ok {
		t.Fatalf("response_meta was visible to public caller: %#v", public)
	}

	worker := shape.ProjectForCaller(fields, responseWorkerCaller)
	if _, ok := worker[ResponseMetaName]; !ok {
		t.Fatalf("response_meta was hidden from worker caller: %#v", worker)
	}
}

func TestResponseMetaFieldHasDistinctEnvelopeType(t *testing.T) {
	shape := Envelope(
		Meta(),
		OptionalField("metadata", MapOf(Any[any]())),
	)

	_ = findAttributeSpec(t, Describe(shape), ResponseMetaName)
	_ = findAttributeSpec(t, Describe(shape), "metadata")
}

func httptestRequest(t *testing.T) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "/orders", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}
