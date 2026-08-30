package response

import (
	"fmt"
	"slices"
	"strings"
	"time"

	callerpkg "github.com/zebodotdev/httpapi/caller"
)

// Type identifies the JSON type emitted by a response shape.
type Type string

const (
	// TypeAny describes a response attribute with no narrower declared type.
	TypeAny Type = "any"

	// TypeArray describes a JSON array response attribute.
	TypeArray Type = "array"

	// TypeBool describes a JSON boolean response attribute.
	TypeBool Type = "boolean"

	// TypeFloat64 describes a JSON number response attribute.
	TypeFloat64 Type = "number"

	// TypeInt describes a JSON integer response attribute.
	TypeInt Type = "integer"

	// TypeInt64 describes a JSON integer response attribute.
	TypeInt64 Type = "integer"

	// TypeObject describes a JSON object response attribute.
	TypeObject Type = "object"

	// TypeString describes a JSON string response attribute.
	TypeString Type = "string"

	// TypeTime describes a JSON string timestamp response attribute.
	TypeTime Type = "string"
)

// ShapeSpec is the transcribable description of a response shape.
//
// ShapeSpec is deliberately provider-neutral. OpenAPI and gateway writers can
// translate it into their own schema objects without importing runtime response
// internals or re-reading application serializers.
type ShapeSpec struct {
	// Type is the JSON type emitted by the shape.
	Type Type

	// Format is the optional OpenAPI-compatible format emitted by the shape.
	Format string

	// Enum is the closed set of string values emitted by the shape.
	Enum []string

	// Attributes describes object attributes when Type is TypeObject.
	Attributes []AttributeSpec

	// Item describes array items when Type is TypeArray.
	Item *ShapeSpec

	// MapValue describes map values when Type is TypeObject and the object has
	// arbitrary string keys.
	MapValue *ShapeSpec
}

// Shape describes the JSON value emitted by a response attribute.
//
// Shapes are used at render time to project caller-aware response bodies and at
// transcription time to expose the same response contract to OpenAPI writers.
type Shape[T any] interface {
	describeShape() ShapeSpec
	projectShape(callerpkg.Caller, T) any
}

type scalarShape[T any] struct {
	typ    Type
	format string
}

func (shape scalarShape[T]) describeShape() ShapeSpec {
	return ShapeSpec{Type: shape.typ, Format: shape.format}
}

func (shape scalarShape[T]) projectShape(_ callerpkg.Caller, value T) any {
	return value
}

// String emits a JSON string attribute.
func String() Shape[string] { return scalarShape[string]{typ: TypeString} }

// Enum emits a JSON string attribute from a closed set of values and exposes
// that set to response contract transcribers.
func Enum(values ...string) Shape[string] {
	return enumShape{values: normalizeEnumValues(values)}
}

type enumShape struct {
	values []string
}

func (shape enumShape) describeShape() ShapeSpec {
	return ShapeSpec{Type: TypeString, Enum: slices.Clone(shape.values)}
}

func (shape enumShape) projectShape(_ callerpkg.Caller, value string) any {
	return value
}

func normalizeEnumValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			panic("httpapi/response: enum value cannot be empty")
		}
		if seen[value] {
			panic(fmt.Sprintf("httpapi/response: duplicate enum value %q", value))
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		panic("httpapi/response: at least one enum value is required")
	}
	return normalized
}

// Int emits a JSON integer attribute.
func Int() Shape[int] { return scalarShape[int]{typ: TypeInt, format: "int32"} }

// Int64 emits a JSON integer attribute from an int64 value.
func Int64() Shape[int64] { return scalarShape[int64]{typ: TypeInt64, format: "int64"} }

// Float64 emits a JSON number attribute.
func Float64() Shape[float64] { return scalarShape[float64]{typ: TypeFloat64, format: "double"} }

// Bool emits a JSON boolean attribute.
func Bool() Shape[bool] { return scalarShape[bool]{typ: TypeBool} }

// Time emits a JSON timestamp attribute.
//
// The standard JSON encoder formats the value with time.Time's
// RFC3339-compatible JSON representation. Spec writers can treat this as a
// string and apply their target's timestamp format convention.
func Time() Shape[time.Time] { return scalarShape[time.Time]{typ: TypeTime, format: "date-time"} }

// Any emits a JSON attribute without additional shaping.
//
// Prefer narrower shapes when possible. Any is best for intentionally flexible
// blobs such as custom metadata where the service does not promise a fixed
// object layout.
func Any[T any]() Shape[T] { return scalarShape[T]{typ: TypeAny} }

// MapOf emits a JSON object with arbitrary string keys and shaped values.
//
// MapOf preserves the source map keys and projects each value through
// valueShape for the active caller. Use it for stable map-shaped contracts
// where every value has the same response shape.
func MapOf[V any](valueShape Shape[V]) Shape[map[string]V] {
	if valueShape == nil {
		panic("httpapi/response: map value shape is required")
	}
	return mapOfShape[V]{valueShape: valueShape}
}

type mapOfShape[V any] struct {
	valueShape Shape[V]
}

func (shape mapOfShape[V]) describeShape() ShapeSpec {
	value := shape.valueShape.describeShape()
	return ShapeSpec{Type: TypeObject, MapValue: &value}
}

func (shape mapOfShape[V]) projectShape(caller callerpkg.Caller, values map[string]V) any {
	projected := make(map[string]any, len(values))
	for key, value := range values {
		projected[key] = shape.valueShape.projectShape(caller, value)
	}
	return projected
}

// Array emits a JSON array whose items do not need caller-aware projection.
//
// Use Array for primitive or already-public item values. Use ArrayOf when each
// item has its own response shape or caller-sensitive attributes.
func Array[T any]() Shape[[]T] { return arrayShape[T]{} }

type arrayShape[T any] struct{}

func (shape arrayShape[T]) describeShape() ShapeSpec {
	return ShapeSpec{Type: TypeArray, Item: &ShapeSpec{Type: TypeAny}}
}

func (shape arrayShape[T]) projectShape(_ callerpkg.Caller, values []T) any {
	return values
}

// ArrayOf emits a JSON array and projects each item through itemShape.
//
// ArrayOf is the collection equivalent of ObjectShape.Body: each item is
// projected for the active caller before the final JSON response is encoded.
func ArrayOf[T any](itemShape Shape[T]) Shape[[]T] {
	if itemShape == nil {
		panic("httpapi/response: array item shape is required")
	}
	return arrayOfShape[T]{itemShape: itemShape}
}

type arrayOfShape[T any] struct {
	itemShape Shape[T]
}

func (shape arrayOfShape[T]) describeShape() ShapeSpec {
	item := shape.itemShape.describeShape()
	return ShapeSpec{Type: TypeArray, Item: &item}
}

func (shape arrayOfShape[T]) projectShape(caller callerpkg.Caller, values []T) any {
	projected := make([]any, 0, len(values))
	for _, value := range values {
		projected = append(projected, shape.itemShape.projectShape(caller, value))
	}
	return projected
}

// Describe returns the transcribable description of shape.
//
// Runtime handlers usually do not need Describe. It exists for tests, docs, and
// transcribers that need to turn response contracts into schema documents.
func Describe[T any](shape Shape[T]) ShapeSpec {
	if shape == nil {
		panic("httpapi/response: shape is required")
	}
	return shape.describeShape()
}
