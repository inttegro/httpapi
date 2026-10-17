package response

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestEnvelopeProjectsNamedFields(t *testing.T) {
	shape := Envelope(
		RequiredField("record", recordResponseShape()),
		OptionalField("error", String()),
	)

	got := shape.ProjectForCaller(Fields(
		Field("record", sampleRecordResponse()),
	), responseWorkerCaller)

	record := got["record"].(map[string]any)
	if record["id"] != "rec_123" || record["created_from"] != "worker" {
		t.Fatalf("record projection = %#v", record)
	}
	if _, ok := got["error"]; ok {
		t.Fatalf("absent optional error was emitted: %#v", got)
	}
}

func TestEnvelopeOmitsUnavailableFields(t *testing.T) {
	shape := Envelope(
		RequiredField("record", recordResponseShape()),
		OptionalField("internal_note", String()).AvailableTo(responseWorkerCaller),
	)

	got := shape.ProjectForCaller(Fields(
		Field("record", sampleRecordResponse()),
		Field("internal_note", "visible to worker"),
	), responsePublicCaller)

	if _, ok := got["internal_note"]; ok {
		t.Fatalf("internal_note was visible to public caller: %#v", got)
	}

	got = shape.ProjectForCaller(Fields(
		Field("record", sampleRecordResponse()),
		Field("internal_note", "visible to worker"),
	), responseWorkerCaller)

	if got["internal_note"] != "visible to worker" {
		t.Fatalf("internal_note = %#v, want visible to worker", got["internal_note"])
	}
}

func TestEnvelopeShapeDescribesAttributes(t *testing.T) {
	spec := Describe(Envelope(
		RequiredField("record", recordResponseShape()),
		OptionalField("internal_note", String()).AvailableTo(responseWorkerCaller),
	))

	if spec.Type != TypeObject {
		t.Fatalf("type = %q, want object", spec.Type)
	}
	if len(spec.Attributes) != 2 {
		t.Fatalf("attributes = %d, want 2", len(spec.Attributes))
	}

	record := findAttributeSpec(t, spec, "record")
	if !record.Required {
		t.Fatal("record was not marked required")
	}
	if record.Shape.Type != TypeObject {
		t.Fatalf("record type = %q, want object", record.Shape.Type)
	}

	internalNote := findAttributeSpec(t, spec, "internal_note")
	if internalNote.Required {
		t.Fatal("internal_note was marked required")
	}
	if !internalNote.Availability.Restricted() {
		t.Fatal("internal_note availability is unrestricted")
	}
}

func TestEnvelopeBodyRendersThroughJSONEncoding(t *testing.T) {
	res := JSON(http.StatusOK, Envelope(
		RequiredField("record", recordResponseShape()),
	).Body(
		Field("record", sampleRecordResponse()),
	))

	body, err := EncodeResponseBodyForCaller(res, responsePublicCaller)
	if err != nil {
		t.Fatalf("EncodeResponseBodyForCaller error = %v", err)
	}

	bodyText := string(body)
	if !strings.Contains(bodyText, `"record"`) || !strings.Contains(bodyText, `"id":"rec_123"`) {
		t.Fatalf("body = %q, want projected envelope", bodyText)
	}
	if strings.Contains(bodyText, "created_from") {
		t.Fatalf("body = %q, created_from should be hidden from public caller", bodyText)
	}
}

func TestEnvelopeBodyAcceptsFieldsDirectly(t *testing.T) {
	shape := Envelope(
		RequiredField("record", recordResponseShape()),
		OptionalField("error", String()),
	)

	got := shape.Body(Field("record", sampleRecordResponse())).
		ProjectResponse(responseWorkerCaller).(map[string]any)

	record := got["record"].(map[string]any)
	if record["id"] != "rec_123" {
		t.Fatalf("record id = %#v, want rec_123", record["id"])
	}
	if _, ok := got["error"]; ok {
		t.Fatalf("absent optional error was emitted: %#v", got)
	}
}

func TestEnvelopeProjectsDistinctGoTypesWithSameJSONShape(t *testing.T) {
	type primaryLabel string
	type secondaryLabel string
	type itemCount int
	type itemLimit int

	primaryLabelShape := Project(String(), func(value primaryLabel) string { return string(value) })
	secondaryLabelShape := Project(String(), func(value secondaryLabel) string { return string(value) })
	itemCountShape := Project(Int(), func(value itemCount) int { return int(value) })
	itemLimitShape := Project(Int(), func(value itemLimit) int { return int(value) })
	shape := Envelope(
		RequiredField("primary_label", primaryLabelShape),
		RequiredField("secondary_label", secondaryLabelShape),
		RequiredField("count", itemCountShape),
		RequiredField("limit", itemLimitShape),
	)

	got := shape.ProjectForCaller(Fields(
		Field("secondary_label", secondaryLabel("settled")),
		Field("primary_label", primaryLabel("pending")),
		Field("limit", itemLimit(10)),
		Field("count", itemCount(3)),
	), responseWorkerCaller)

	if got["primary_label"] != "pending" {
		t.Fatalf("primary_label = %#v, want pending", got["primary_label"])
	}
	if got["secondary_label"] != "settled" {
		t.Fatalf("secondary_label = %#v, want settled", got["secondary_label"])
	}
	if got["count"] != 3 {
		t.Fatalf("count = %#v, want 3", got["count"])
	}
	if got["limit"] != 10 {
		t.Fatalf("limit = %#v, want 10", got["limit"])
	}
}

func TestEnvelopeOmitsOptionalNilFields(t *testing.T) {
	t.Run("typed nil pointer with visible field", func(t *testing.T) {
		shape := Envelope(
			OptionalField("record", Any[*recordResponse]()),
			OptionalField("status", String()),
		)

		got := shape.ProjectForCaller(Fields(
			Field("record", (*recordResponse)(nil)),
			Field("status", "ready"),
		), responseWorkerCaller)

		if _, ok := got["record"]; ok {
			t.Fatalf("typed nil optional field was emitted: %#v", got)
		}
		if got["status"] != "ready" {
			t.Fatalf("status = %#v, want ready", got["status"])
		}
	})

	t.Run("nil interface with visible field", func(t *testing.T) {
		shape := Envelope(
			OptionalField("metadata", Any[any]()),
			OptionalField("status", String()),
		)

		got := shape.ProjectForCaller(Fields(
			Field[any]("metadata", nil),
			Field("status", "ready"),
		), responseWorkerCaller)

		if _, ok := got["metadata"]; ok {
			t.Fatalf("nil interface optional field was emitted: %#v", got)
		}
		if got["status"] != "ready" {
			t.Fatalf("status = %#v, want ready", got["status"])
		}
	})

	t.Run("nil slice with visible field", func(t *testing.T) {
		shape := Envelope(
			OptionalField("tags", Array[string]()),
			OptionalField("status", String()),
		)

		got := shape.ProjectForCaller(Fields(
			Field("tags", []string(nil)),
			Field("status", "ready"),
		), responseWorkerCaller)

		if _, ok := got["tags"]; ok {
			t.Fatalf("nil slice optional field was emitted: %#v", got)
		}
		if got["status"] != "ready" {
			t.Fatalf("status = %#v, want ready", got["status"])
		}
	})

	t.Run("empty nonnil slice is real", func(t *testing.T) {
		shape := Envelope(
			OptionalField("tags", Array[string]()),
		)

		got := shape.ProjectForCaller(Fields(
			Field("tags", []string{}),
		), responseWorkerCaller)

		tags, ok := got["tags"].([]string)
		if !ok {
			t.Fatalf("tags = %T, want []string", got["tags"])
		}
		if len(tags) != 0 {
			t.Fatalf("tags length = %d, want 0", len(tags))
		}
	})
}

func TestEnvelopeCanShapeNestedStructlessObjects(t *testing.T) {
	type pageNumber int
	type pageSize int

	pageNumberShape := Project(Int(), func(value pageNumber) int { return int(value) })
	pageSizeShape := Project(Int(), func(value pageSize) int { return int(value) })
	pageShape := Envelope(
		RequiredField("number", pageNumberShape),
		RequiredField("size", pageSizeShape),
		RequiredField("records", ArrayOf(recordResponseShape())),
	)
	responseShape := Envelope(
		RequiredField("page", pageShape),
	)

	got := responseShape.Body(Field("page", Fields(
		Field("number", pageNumber(2)),
		Field("size", pageSize(50)),
		Field("records", []recordResponse{sampleRecordResponse()}),
	))).ProjectResponse(responseWorkerCaller).(map[string]any)

	page := got["page"].(map[string]any)
	if page["number"] != 2 || page["size"] != 50 {
		t.Fatalf("page = %#v, want number 2 and size 50", page)
	}
	records := page["records"].([]any)
	record := records[0].(map[string]any)
	if record["id"] != "rec_123" || record["created_from"] != "worker" {
		t.Fatalf("nested record = %#v", record)
	}
}

func TestEnvelopePanicsForMissingRequiredField(t *testing.T) {
	shape := Envelope(RequiredField("record", recordResponseShape()))

	defer func() {
		if recover() == nil {
			t.Fatal("expected missing required field panic")
		}
	}()

	shape.ProjectForCaller(Fields(), responseWorkerCaller)
}

func TestEnvelopeInvariantFieldsRequireExactlyOneAlternative(t *testing.T) {
	shape := Envelope(
		InvariantField("record", recordResponseShape()),
		InvariantField("status", String()),
	)

	record := shape.Body(Field("record", sampleRecordResponse())).
		ProjectResponse(responseWorkerCaller).(map[string]any)
	if _, ok := record["record"]; !ok {
		t.Fatalf("record invariant was not projected: %#v", record)
	}

	status := shape.Body(Field("status", "ready")).
		ProjectResponse(responseWorkerCaller).(map[string]any)
	if status["status"] != "ready" {
		t.Fatalf("status invariant = %#v, want ready", status["status"])
	}

	for _, fields := range [][]EnvelopeField{
		nil,
		{Field("record", sampleRecordResponse()), Field("status", "ready")},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected invariant panic for fields %#v", fields)
				}
			}()
			shape.ProjectForCaller(Fields(fields...), responseWorkerCaller)
		}()
	}

	spec := Describe(shape)
	if len(spec.Rules) != 1 {
		t.Fatalf("invariant rules = %#v, want one", spec.Rules)
	}
	rule := spec.Rules[0]
	if !reflect.DeepEqual(rule.Names, []string{"record", "status"}) ||
		rule.MinPresent != 1 || rule.MaxPresent != 1 {
		t.Fatalf("invariant rule = %#v", rule)
	}
}

func TestEnvelopePanicsForWrongFieldType(t *testing.T) {
	shape := Envelope(RequiredField("record", recordResponseShape()))

	defer func() {
		if recover() == nil {
			t.Fatal("expected wrong field type panic")
		}
	}()

	shape.ProjectForCaller(Fields(Field("record", "not a record")), responseWorkerCaller)
}

func TestEnvelopePanicsForUnexpectedField(t *testing.T) {
	shape := Envelope(OptionalField("record", recordResponseShape()))

	defer func() {
		if recover() == nil {
			t.Fatal("expected unexpected field panic")
		}
	}()

	shape.ProjectForCaller(Fields(Field("recrod", sampleRecordResponse())), responseWorkerCaller)
}

func TestEnvelopePanicsForEmptyDefinitions(t *testing.T) {
	t.Run("constructor", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected empty envelope definition panic")
			}
		}()

		Envelope()
	})

	t.Run("zero value projection", func(t *testing.T) {
		var shape EnvelopeShape

		defer func() {
			if recover() == nil {
				t.Fatal("expected zero value envelope projection panic")
			}
		}()

		shape.ProjectForCaller(Fields(), responseWorkerCaller)
	})

	t.Run("zero value description", func(t *testing.T) {
		var shape EnvelopeShape

		defer func() {
			if recover() == nil {
				t.Fatal("expected zero value envelope description panic")
			}
		}()

		shape.Attributes()
	})
}

func TestEnvelopePanicsWhenProjectionEmitsNoFields(t *testing.T) {
	t.Run("no values", func(t *testing.T) {
		shape := Envelope(OptionalField("record", recordResponseShape()))

		defer func() {
			if recover() == nil {
				t.Fatal("expected empty projection panic")
			}
		}()

		shape.ProjectForCaller(Fields(), responseWorkerCaller)
	})

	t.Run("nil values", func(t *testing.T) {
		shape := Envelope(OptionalField("record", recordResponseShape()))

		defer func() {
			if recover() == nil {
				t.Fatal("expected empty projection panic")
			}
		}()

		shape.ProjectForCaller(EnvelopeValues{}, responseWorkerCaller)
	})

	t.Run("only optional nil value", func(t *testing.T) {
		shape := Envelope(OptionalField("value", Any[any]()))

		defer func() {
			if recover() == nil {
				t.Fatal("expected empty projection panic")
			}
		}()

		shape.ProjectForCaller(Fields(Field[any]("value", nil)), responseWorkerCaller)
	})

	t.Run("all values unavailable", func(t *testing.T) {
		shape := Envelope(
			OptionalField("internal_note", String()).AvailableTo(responseWorkerCaller),
		)

		defer func() {
			if recover() == nil {
				t.Fatal("expected empty projection panic")
			}
		}()

		shape.ProjectForCaller(Fields(
			Field("internal_note", "visible to worker"),
		), responsePublicCaller)
	})

	t.Run("all values nil", func(t *testing.T) {
		shape := Envelope(
			OptionalField("record", Any[*recordResponse]()),
			OptionalField("tags", Array[string]()),
		)

		defer func() {
			if recover() == nil {
				t.Fatal("expected empty projection panic")
			}
		}()

		shape.ProjectForCaller(Fields(
			Field("record", (*recordResponse)(nil)),
			Field("tags", []string(nil)),
		), responseWorkerCaller)
	})

	t.Run("render", func(t *testing.T) {
		res := JSON(http.StatusOK, Envelope(
			OptionalField("record", recordResponseShape()),
		).Body())

		defer func() {
			if recover() == nil {
				t.Fatal("expected empty render projection panic")
			}
		}()

		_, _ = EncodeResponseBodyForCaller(res, responseWorkerCaller)
	})
}

func TestEnvelopeOptionalFieldsOmitNilValues(t *testing.T) {
	t.Run("untyped nil", func(t *testing.T) {
		shape := Envelope(
			OptionalField("value", Any[any]()),
			RequiredField("fallback", String()),
		)

		got := shape.ProjectForCaller(Fields(
			Field[any]("value", nil),
			Field("fallback", "ok"),
		), responseWorkerCaller)

		if _, ok := got["value"]; ok {
			t.Fatalf("nil optional value was emitted: %#v", got)
		}
		if got["fallback"] != "ok" {
			t.Fatalf("fallback = %#v, want ok", got["fallback"])
		}
	})

	t.Run("typed nil pointer", func(t *testing.T) {
		shape := Envelope(
			OptionalField("record", Any[*recordResponse]()),
			RequiredField("fallback", String()),
		)
		var record *recordResponse

		got := shape.ProjectForCaller(Fields(
			Field("record", record),
			Field("fallback", "ok"),
		), responseWorkerCaller)

		if _, ok := got["record"]; ok {
			t.Fatalf("typed nil pointer optional value was emitted: %#v", got)
		}
	})

	t.Run("nil map and slice", func(t *testing.T) {
		shape := Envelope(
			OptionalField("metadata", MapOf(String())),
			OptionalField("tags", Array[string]()),
			RequiredField("fallback", String()),
		)
		var metadata map[string]string
		var tags []string

		got := shape.ProjectForCaller(Fields(
			Field("metadata", metadata),
			Field("tags", tags),
			Field("fallback", "ok"),
		), responseWorkerCaller)

		if _, ok := got["metadata"]; ok {
			t.Fatalf("nil map optional value was emitted: %#v", got)
		}
		if _, ok := got["tags"]; ok {
			t.Fatalf("nil slice optional value was emitted: %#v", got)
		}
	})
}

func TestEnvelopeOptionalFieldsCountNonNilEmptyCollections(t *testing.T) {
	shape := Envelope(
		OptionalField("metadata", MapOf(String())),
		OptionalField("tags", Array[string]()),
	)

	got := shape.ProjectForCaller(Fields(
		Field("metadata", map[string]string{}),
		Field("tags", []string{}),
	), responseWorkerCaller)

	if metadata, ok := got["metadata"].(map[string]any); !ok || len(metadata) != 0 {
		t.Fatalf("metadata = %#v, want empty projected map", got["metadata"])
	}
	if tags, ok := got["tags"].([]string); !ok || len(tags) != 0 {
		t.Fatalf("tags = %#v, want empty slice", got["tags"])
	}
}

func TestEnvelopePanicsForRequiredNilValues(t *testing.T) {
	tests := []struct {
		name  string
		shape *EnvelopeShape
		field EnvelopeField
	}{
		{
			name:  "untyped nil",
			shape: Envelope(RequiredField("value", Any[any]())),
			field: Field[any]("value", nil),
		},
		{
			name:  "typed nil pointer",
			shape: Envelope(RequiredField("record", Any[*recordResponse]())),
			field: Field("record", (*recordResponse)(nil)),
		},
		{
			name:  "nil map",
			shape: Envelope(RequiredField("metadata", MapOf(String()))),
			field: Field("metadata", map[string]string(nil)),
		},
		{
			name:  "nil slice",
			shape: Envelope(RequiredField("tags", Array[string]())),
			field: Field("tags", []string(nil)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected required nil value panic")
				}
			}()

			tt.shape.ProjectForCaller(Fields(tt.field), responseWorkerCaller)
		})
	}
}

func TestEnvelopePanicsForDuplicateAttributesAndFields(t *testing.T) {
	t.Run("attributes", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected duplicate attribute panic")
			}
		}()

		Envelope(
			OptionalField("record", recordResponseShape()),
			OptionalField("record", String()),
		)
	})

	t.Run("attribute types", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected duplicate attribute type panic")
			}
		}()

		Envelope(
			OptionalField("primary_label", String()),
			OptionalField("secondary_label", String()),
		)
	})

	t.Run("attribute types through constructor", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected duplicate attribute type panic")
			}
		}()

		Envelope(
			OptionalField("count", Int()),
			OptionalField("limit", Int()),
		)
	})

	t.Run("fields", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected duplicate field panic")
			}
		}()

		Fields(
			Field("record", sampleRecordResponse()),
			Field("record", sampleRecordResponse()),
		)
	})

	t.Run("zero field", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected empty field name panic")
			}
		}()

		Fields(EnvelopeField{})
	})
}
