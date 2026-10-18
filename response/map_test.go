package response

import (
	"testing"
)

func TestMapOfProjectsValuesInContext(t *testing.T) {
	shape := MapOf(detailResponseShape())
	values := map[string]detailResponse{
		"primary": {
			Status:       "ready",
			InternalRef:  "ref_primary",
			InternalSeen: true,
		},
		"secondary": {
			Status:       "pending",
			InternalRef:  "ref_secondary",
			InternalSeen: true,
		},
	}

	got := Body(shape, values).Project(Context{Caller: responsePublicCaller}).(map[string]any)

	primary := got["primary"].(map[string]any)
	if primary["status"] != "ready" {
		t.Fatalf("primary status = %#v, want ready", primary["status"])
	}
	if _, ok := primary["internal_ref"]; ok {
		t.Fatalf("primary internal_ref was visible to public caller: %#v", primary)
	}

	secondary := got["secondary"].(map[string]any)
	if secondary["status"] != "pending" {
		t.Fatalf("secondary status = %#v, want pending", secondary["status"])
	}
	if _, ok := secondary["internal_ref"]; ok {
		t.Fatalf("secondary internal_ref was visible to public caller: %#v", secondary)
	}

	got = Body(shape, values).Project(Context{Caller: responseWorkerCaller}).(map[string]any)

	primary = got["primary"].(map[string]any)
	if primary["internal_ref"] != "ref_primary" {
		t.Fatalf("primary internal_ref = %#v, want ref_primary", primary["internal_ref"])
	}
	secondary = got["secondary"].(map[string]any)
	if secondary["internal_ref"] != "ref_secondary" {
		t.Fatalf("secondary internal_ref = %#v, want ref_secondary", secondary["internal_ref"])
	}
}

func TestMapOfShapeDescribesValueShape(t *testing.T) {
	spec := Describe(MapOf(detailResponseShape()))

	if spec.Type != TypeObject {
		t.Fatalf("type = %q, want object", spec.Type)
	}
	if len(spec.Attributes) != 0 {
		t.Fatalf("attributes = %#v, want none", spec.Attributes)
	}
	if spec.MapValue == nil {
		t.Fatal("map value shape is nil")
	}
	if spec.MapValue.Type != TypeObject {
		t.Fatalf("map value type = %q, want object", spec.MapValue.Type)
	}

	status := findAttributeSpec(t, *spec.MapValue, "status")
	if status.Shape.Type != TypeString {
		t.Fatalf("status type = %q, want string", status.Shape.Type)
	}

	internalRef := findAttributeSpec(t, *spec.MapValue, "internal_ref")
	if !internalRef.Availability.Restricted() {
		t.Fatal("internal_ref availability is unrestricted")
	}
}

func TestMapOfPanicsWithoutValueShape(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected nil value shape panic")
		}
	}()

	MapOf[detailResponse](nil)
}
