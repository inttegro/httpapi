package param

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

// Shape describes the JSON wire value accepted by a parameter.
//
// Shape has unexported methods so request parsers can only be composed from the
// shapes provided by this package.
type Shape[T any] interface {
	describeShape() ShapeSpec
	parseShape(parseContext, string, any) (T, *Error)
	wireType() Type
}

type parseContext struct {
	config parseConfig
}

type scalarShape[T any] struct {
	typ    Type
	parser func(string, any) (T, *Error)
}

// String accepts JSON string values.
func String() Shape[string] {
	return scalarShape[string]{
		typ: TypeString,
		parser: func(path string, raw any) (string, *Error) {
			value, ok := raw.(string)
			if !ok {
				return "", typeMismatch(path, TypeString)
			}
			return value, nil
		},
	}
}

// Int accepts JSON integer values representable as int.
func Int() Shape[int] {
	return scalarShape[int]{
		typ: TypeInt,
		parser: func(path string, raw any) (int, *Error) {
			value, ok := rawInt(raw)
			if !ok {
				return 0, typeMismatch(path, TypeInt)
			}
			return value, nil
		},
	}
}

// Int64 accepts JSON integer values representable as int64.
func Int64() Shape[int64] {
	return scalarShape[int64]{
		typ: TypeInt64,
		parser: func(path string, raw any) (int64, *Error) {
			value, ok := rawInt64(raw)
			if !ok {
				return 0, typeMismatch(path, TypeInt64)
			}
			return value, nil
		},
	}
}

// Float64 accepts JSON number values representable as float64.
func Float64() Shape[float64] {
	return scalarShape[float64]{
		typ: TypeFloat64,
		parser: func(path string, raw any) (float64, *Error) {
			value, ok := rawFloat64(raw)
			if !ok {
				return 0, typeMismatch(path, TypeFloat64)
			}
			return value, nil
		},
	}
}

// Bool accepts JSON boolean values.
func Bool() Shape[bool] {
	return scalarShape[bool]{
		typ: TypeBool,
		parser: func(path string, raw any) (bool, *Error) {
			value, ok := raw.(bool)
			if !ok {
				return false, typeMismatch(path, TypeBool)
			}
			return value, nil
		},
	}
}

// Any accepts any non-null JSON value. Combine it with NullAccepted when null
// is meaningful for the endpoint.
func Any() Shape[any] {
	return scalarShape[any]{
		typ: TypeAny,
		parser: func(_ string, raw any) (any, *Error) {
			return raw, nil
		},
	}
}

func (shape scalarShape[T]) parseShape(_ parseContext, path string, raw any) (T, *Error) {
	return shape.parser(path, raw)
}

func (shape scalarShape[T]) describeShape() ShapeSpec {
	return ShapeSpec{Type: shape.typ}
}

func (shape scalarShape[T]) wireType() Type {
	return shape.typ
}

// Array accepts JSON array values and decodes each item as T.
func Array[T any]() Shape[[]T] {
	return arrayShape[T]{}
}

type arrayShape[T any] struct{}

func (shape arrayShape[T]) parseShape(_ parseContext, path string, raw any) ([]T, *Error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, typeMismatch(path, TypeArray)
	}

	result := make([]T, 0, len(items))
	for idx, item := range items {
		value, err := decodeJSONLike[T](item)
		if err != nil {
			itemPath := fmt.Sprintf("%s[%d]", path, idx)
			return nil, parserError(itemPath, err)
		}
		result = append(result, value)
	}
	return result, nil
}

func (shape arrayShape[T]) describeShape() ShapeSpec {
	return ShapeSpec{Type: TypeArray, Item: &ShapeSpec{Type: TypeAny}}
}

func (shape arrayShape[T]) wireType() Type {
	return TypeArray
}

// ArrayOf accepts JSON array values and parses each item with itemShape.
func ArrayOf[T any](itemShape Shape[T]) Shape[[]T] {
	if itemShape == nil {
		panic("httpapi/param: array item shape is required")
	}
	return arrayOfShape[T]{itemShape: itemShape}
}

type arrayOfShape[T any] struct {
	itemShape Shape[T]
}

func (shape arrayOfShape[T]) parseShape(ctx parseContext, path string, raw any) ([]T, *Error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, typeMismatch(path, TypeArray)
	}

	result := make([]T, 0, len(items))
	for idx, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", path, idx)
		value, err := shape.itemShape.parseShape(ctx, itemPath, item)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (shape arrayOfShape[T]) describeShape() ShapeSpec {
	item := shape.itemShape.describeShape()
	return ShapeSpec{Type: TypeArray, Item: &item}
}

func (shape arrayOfShape[T]) wireType() Type {
	return TypeArray
}

// MapOf accepts JSON objects with arbitrary property names and parses every
// property value with valueShape.
//
// Use Object when the accepted property names are known in advance. MapOf is
// intended for keyed collections whose keys are part of the request data, such
// as named response projections.
func MapOf[T any](valueShape Shape[T]) Shape[map[string]T] {
	if valueShape == nil {
		panic("httpapi/param: map value shape is required")
	}
	return mapOfShape[T]{valueShape: valueShape}
}

type mapOfShape[T any] struct {
	valueShape Shape[T]
}

func (shape mapOfShape[T]) parseShape(
	ctx parseContext,
	path string,
	raw any,
) (map[string]T, *Error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, typeMismatch(path, TypeObject)
	}

	result := make(map[string]T, len(object))
	for name, item := range object {
		value, err := shape.valueShape.parseShape(ctx, joinPath(path, name), item)
		if err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}

func (shape mapOfShape[T]) describeShape() ShapeSpec {
	value := shape.valueShape.describeShape()
	return ShapeSpec{Type: TypeObject, MapValue: &value}
}

func (shape mapOfShape[T]) wireType() Type {
	return TypeObject
}

func typeMismatch(path string, typ Type) *Error {
	return paramError(
		path,
		CodeTypeMismatch,
		fmt.Sprintf("`%s` must be %s", path, typeDisplayName(typ)),
		nil,
	)
}

func rawInt(raw any) (int, bool) {
	value, ok := rawInt64(raw)
	if !ok || value < math.MinInt || value > math.MaxInt {
		return 0, false
	}
	return int(value), true
}

func rawInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		if uint64(value) > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case float32:
		return integralFloat64(float64(value))
	case float64:
		return integralFloat64(value)
	default:
		return 0, false
	}
}

func integralFloat64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	if value < math.MinInt64 || value > math.MaxInt64 {
		return 0, false
	}
	if math.Trunc(value) != value {
		return 0, false
	}
	return int64(value), true
}

func rawFloat64(raw any) (float64, bool) {
	switch value := raw.(type) {
	case json.Number:
		parsed, err := strconv.ParseFloat(value.String(), 64)
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float32:
		parsed := float64(value)
		return parsed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}

func decodeJSONLike[T any](raw any) (T, error) {
	if value, ok := raw.(T); ok {
		return value, nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return zeroValue[T](), err
	}

	var value T
	if err := json.Unmarshal(encoded, &value); err != nil {
		return zeroValue[T](), err
	}
	return value, nil
}

func measuredSize(raw any) (float64, string, bool) {
	switch value := raw.(type) {
	case string:
		return float64(len(value)), "byte", true
	case int:
		return float64(value), "value", true
	case int64:
		return float64(value), "value", true
	case float64:
		return value, "value", true
	case []any:
		return float64(len(value)), "item", true
	case map[string]any:
		return float64(len(value)), "field", true
	default:
		reflected := reflect.ValueOf(raw)
		switch reflected.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return float64(reflected.Len()), "item", true
		default:
			return 0, "", false
		}
	}
}

func describeSize(size int64, unit string) string {
	if unit == "value" {
		return fmt.Sprintf("%d", size)
	}
	if size == 1 {
		return fmt.Sprintf("%d %s", size, unit)
	}
	return fmt.Sprintf("%d %ss", size, unit)
}
