package response

import (
	"testing"

	callerpkg "github.com/inttegro/httpapi/caller"
)

var (
	responsePublicCaller    = callerpkg.Define("public-api")
	responseWorkerCaller    = callerpkg.Define("worker")
	responseDashboardCaller = callerpkg.Define("dashboard")
	responseAdminCaller     = callerpkg.Define("admin")
)

type recordResponse struct {
	ID          string
	Count       int64
	CreatedFrom string
	Detail      detailResponse
	Tags        []tagResponse
}

type detailResponse struct {
	Status       string
	InternalRef  string
	InternalSeen bool
}

type tagResponse struct {
	Name     string
	Reserved bool
}

type preparedRecordResponse struct {
	record recordResponse
}

func (prepared preparedRecordResponse) ID() string {
	return prepared.record.ID
}

func (prepared preparedRecordResponse) CreatedFrom() (string, bool) {
	return prepared.record.CreatedFrom, prepared.record.CreatedFrom != ""
}

func TestObjectProjectOmitsUnavailableAttributes(t *testing.T) {
	shape := recordResponseShape()
	record := sampleRecordResponse()

	got := shape.Project(record, Context{Caller: responsePublicCaller})

	if got["id"] != "rec_123" || got["count"] != int64(7) {
		t.Fatalf("projected public fields = %#v", got)
	}
	if _, ok := got["created_from"]; ok {
		t.Fatalf("created_from was visible to public caller: %#v", got)
	}
	detail := got["detail"].(map[string]any)
	if _, ok := detail["internal_ref"]; ok {
		t.Fatalf("nested internal_ref was visible to public caller: %#v", detail)
	}
	tags := got["tags"].([]any)
	tag := tags[0].(map[string]any)
	if _, ok := tag["reserved"]; ok {
		t.Fatalf("nested array reserved flag was visible to public caller: %#v", tag)
	}
}

func TestObjectProjectIncludesAvailableAttributes(t *testing.T) {
	shape := recordResponseShape()
	record := sampleRecordResponse()

	got := shape.Project(record, Context{Caller: responseWorkerCaller})

	if got["created_from"] != "worker" {
		t.Fatalf("created_from = %#v, want worker", got["created_from"])
	}
	detail := got["detail"].(map[string]any)
	if detail["internal_ref"] != "ref_123" {
		t.Fatalf("internal_ref = %#v, want ref_123", detail["internal_ref"])
	}
	tags := got["tags"].([]any)
	tag := tags[0].(map[string]any)
	if tag["reserved"] != true {
		t.Fatalf("reserved = %#v, want true", tag["reserved"])
	}
}

func TestBodyProjectsWithAnyResponseShape(t *testing.T) {
	body := Body(recordResponseShape(), sampleRecordResponse())

	got := body.Project(Context{Caller: responsePublicCaller}).(map[string]any)

	if got["id"] != "rec_123" {
		t.Fatalf("id = %#v, want rec_123", got["id"])
	}
	if _, ok := got["created_from"]; ok {
		t.Fatalf("created_from was visible to public caller: %#v", got)
	}
}

func TestObjectShapeDescribesAttributes(t *testing.T) {
	spec := Describe(recordResponseShape())

	if spec.Type != TypeObject {
		t.Fatalf("type = %q, want object", spec.Type)
	}
	if len(spec.Attributes) != 5 {
		t.Fatalf("attributes = %d, want 5", len(spec.Attributes))
	}

	createdFrom := findAttributeSpec(t, spec, "created_from")
	if createdFrom.Required {
		t.Fatal("created_from reported required")
	}
	if !createdFrom.Availability.Restricted() {
		t.Fatal("created_from availability is unrestricted")
	}
	if !createdFrom.Availability.Allows(responseWorkerCaller) {
		t.Fatal("created_from availability does not allow worker")
	}
	if createdFrom.Availability.Allows(responsePublicCaller) {
		t.Fatal("created_from availability allows public caller")
	}
	if createdFrom.Shape.Type != TypeString {
		t.Fatalf("created_from type = %q, want string", createdFrom.Shape.Type)
	}

	tags := findAttributeSpec(t, spec, "tags")
	if tags.Shape.Type != TypeArray || tags.Shape.Item == nil || tags.Shape.Item.Type != TypeObject {
		t.Fatalf("tags shape = %#v, want array of object", tags.Shape)
	}
}

func TestObjectProjectOmitsAbsentOptionalAttributes(t *testing.T) {
	shape := recordResponseShape()
	record := sampleRecordResponse()
	record.CreatedFrom = ""
	record.Detail.InternalSeen = false

	got := shape.Project(record, Context{Caller: responseWorkerCaller})

	if _, ok := got["created_from"]; ok {
		t.Fatalf("absent created_from was emitted: %#v", got)
	}
	detail := got["detail"].(map[string]any)
	if _, ok := detail["internal_ref"]; ok {
		t.Fatalf("absent internal_ref was emitted: %#v", detail)
	}
}

func TestObjectShapePanicsForDuplicateAttributes(t *testing.T) {
	shape := Object[recordResponse]().
		Attribute(Required("id", String(), func(record recordResponse) string {
			return record.ID
		})).
		Attribute(Optional("id", String(), func(record recordResponse) (string, bool) {
			return record.CreatedFrom, record.CreatedFrom != ""
		}))

	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate attribute panic")
		}
	}()

	shape.Project(sampleRecordResponse(), Context{Caller: responseWorkerCaller})
}

func TestProjectPreparesSourceValueOnce(t *testing.T) {
	prepareCalls := 0
	shape := Project(
		Object[preparedRecordResponse](
			Required("id", String(), preparedRecordResponse.ID),
			Optional("created_from", String(), preparedRecordResponse.CreatedFrom).AvailableTo(responseWorkerCaller),
		),
		func(record recordResponse) preparedRecordResponse {
			prepareCalls++
			return preparedRecordResponse{record: record}
		},
	)

	got := shape.Project(sampleRecordResponse(), Context{Caller: responseWorkerCaller}).(map[string]any)

	if got["id"] != "rec_123" || got["created_from"] != "worker" {
		t.Fatalf("prepared projection = %#v", got)
	}
	if prepareCalls != 1 {
		t.Fatalf("prepare calls = %d, want 1", prepareCalls)
	}

	got = shape.Project(sampleRecordResponse(), Context{Caller: responsePublicCaller}).(map[string]any)

	if _, ok := got["created_from"]; ok {
		t.Fatalf("unavailable prepared attribute was visible: %#v", got)
	}
	if prepareCalls != 2 {
		t.Fatalf("prepare calls after second projection = %d, want 2", prepareCalls)
	}

	spec := Describe(shape)
	if spec.Type != TypeObject {
		t.Fatalf("projected shape type = %q, want object", spec.Type)
	}
	if len(spec.Attributes) != 2 {
		t.Fatalf("projected shape attributes = %d, want 2", len(spec.Attributes))
	}
}

func recordResponseShape() *ObjectShape[recordResponse] {
	return Object[recordResponse]().
		Attribute(Required("id", String(), func(record recordResponse) string {
			return record.ID
		})).
		Attribute(Required("count", Int64(), func(record recordResponse) int64 {
			return record.Count
		})).
		Attribute(Optional("created_from", String(), func(record recordResponse) (string, bool) {
			return record.CreatedFrom, record.CreatedFrom != ""
		}).AvailableTo(responseWorkerCaller, responseDashboardCaller, responseAdminCaller)).
		Attribute(Required("detail", detailResponseShape(), func(record recordResponse) detailResponse {
			return record.Detail
		})).
		Attribute(Required("tags", ArrayOf(tagResponseShape()), func(record recordResponse) []tagResponse {
			return record.Tags
		}))
}

func detailResponseShape() *ObjectShape[detailResponse] {
	return Object[detailResponse]().
		Attribute(Required("status", String(), func(detail detailResponse) string {
			return detail.Status
		})).
		Attribute(Optional("internal_ref", String(), func(detail detailResponse) (string, bool) {
			return detail.InternalRef, detail.InternalSeen
		}).AvailableTo(responseWorkerCaller, responseAdminCaller))
}

func tagResponseShape() *ObjectShape[tagResponse] {
	return Object[tagResponse]().
		Attribute(Required("name", String(), func(tag tagResponse) string {
			return tag.Name
		})).
		Attribute(Required("reserved", Bool(), func(tag tagResponse) bool {
			return tag.Reserved
		}).AvailableTo(responseWorkerCaller))
}

func sampleRecordResponse() recordResponse {
	return recordResponse{
		ID:          "rec_123",
		Count:       7,
		CreatedFrom: "worker",
		Detail: detailResponse{
			Status:       "ready",
			InternalRef:  "ref_123",
			InternalSeen: true,
		},
		Tags: []tagResponse{
			{Name: "alpha", Reserved: true},
		},
	}
}

func findAttributeSpec(t *testing.T, spec ShapeSpec, name string) AttributeSpec {
	t.Helper()

	for _, attribute := range spec.Attributes {
		if attribute.Name == name {
			return attribute
		}
	}
	t.Fatalf("attribute %q not found in %#v", name, spec.Attributes)
	return AttributeSpec{}
}
