package param

import (
	"reflect"
	"testing"
)

func TestMapOfParsesArbitraryPropertyValues(t *testing.T) {
	request := JSON[map[string]string]().
		Param(Required("labels", MapOf(String()))).
		Parse(func(values Values) (map[string]string, error) {
			return Must[map[string]string](values, "labels"), nil
		})

	got, err := request.Parse(`{"labels":{"merchant":"name","product":"media.gallery"}}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	want := map[string]string{
		"merchant": "name",
		"product":  "media.gallery",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed map = %#v, want %#v", got, want)
	}

	spec := Describe(request).Body.Parameters[0].Shape
	if spec.Type != TypeObject || spec.MapValue == nil || spec.MapValue.Type != TypeString {
		t.Fatalf("map shape = %#v", spec)
	}
}

func TestMapOfReportsValuePath(t *testing.T) {
	request := JSON[map[string]string]().
		Param(Required("labels", MapOf(String()))).
		Parse(func(values Values) (map[string]string, error) {
			return Must[map[string]string](values, "labels"), nil
		})

	_, err := request.Parse(`{"labels":{"merchant":42}}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Param != "labels.merchant" || err.Code != CodeTypeMismatch {
		t.Fatalf("error = %#v", err)
	}
}
