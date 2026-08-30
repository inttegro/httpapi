package response

import (
	"reflect"
	"testing"
)

func TestEnumDescribesClosedStringValues(t *testing.T) {
	shape := Enum("pending", " succeeded ")
	described := Describe(shape)
	if described.Type != TypeString || !reflect.DeepEqual(described.Enum, []string{"pending", "succeeded"}) {
		t.Fatalf("enum description = %#v", described)
	}

	described.Enum[0] = "mutated"
	if got := Describe(shape).Enum[0]; got != "pending" {
		t.Fatalf("enum description leaked mutation: %q", got)
	}
}

func TestEnumRejectsInvalidDefinitions(t *testing.T) {
	for _, values := range [][]string{nil, {""}, {"pending", " pending "}} {
		t.Run("invalid", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("Enum(%#v) did not panic", values)
				}
			}()
			_ = Enum(values...)
		})
	}
}
