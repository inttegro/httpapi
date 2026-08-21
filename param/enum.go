package param

import (
	"fmt"
	"slices"
	"strings"
)

// Enum accepts JSON string values that exactly match one of values.
func Enum(values ...string) Shape[string] {
	return enumShape{values: normalizeEnumValues(values)}
}

type enumShape struct {
	values    []string
	trimInput bool
}

func (shape enumShape) parseShape(_ parseContext, path string, raw any) (string, *Error) {
	value, ok := raw.(string)
	if !ok {
		return "", typeMismatch(path, TypeString)
	}
	if shape.trimInput {
		value = strings.TrimSpace(value)
	}
	if !stringIn(value, shape.values) {
		return "", valueNotAllowedError(path, value, shape.values)
	}
	return value, nil
}

func (shape enumShape) describeShape() ShapeSpec {
	return ShapeSpec{
		Type: TypeString,
		Enum: cloneStringSlice(shape.values),
	}
}

func (shape enumShape) wireType() Type {
	return TypeString
}

// TrimmedEnum accepts JSON string values after trimming leading and trailing
// whitespace, then validates the resulting value against values.
//
// Use TrimmedEnum for request parameters where the API already treats
// incidental whitespace as input cruft, but the parameter is still a closed
// string set that should appear as an enum in generated contracts.
func TrimmedEnum(values ...string) Shape[string] {
	return enumShape{
		values:    normalizeEnumValues(values),
		trimInput: true,
	}
}

func valueNotAllowedError(path string, value string, allowed []string) *Error {
	return paramError(
		path,
		CodeValueNotAllowed,
		fmt.Sprintf(
			"`%s` must be one of %s; got %q",
			path,
			formatNameList(allowed),
			value,
		),
		nil,
	)
}

func normalizeEnumValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			panic("httpapi/param: enum value cannot be empty")
		}
		if seen[value] {
			panic(fmt.Sprintf("httpapi/param: duplicate enum value %q", value))
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		panic("httpapi/param: at least one enum value is required")
	}
	return normalized
}

func stringIn(value string, values []string) bool {
	return slices.Contains(values, value)
}
