package param

import (
	"fmt"
	"reflect"
	"strings"

	callerpkg "github.com/inttegro/httpapi/caller"
)

// ParamDefinition is a parameter or parameter group declaration that can be
// added to a JSON request or object shape with Param.
//
// Values are normally created with Required or Optional. Parameter groups such
// as MutuallyExclusive can also be added anywhere Param accepts a
// ParamDefinition.
type ParamDefinition interface {
	addToParamSet(paramSet)
}

// AcceptedParam is one accepted request parameter.
//
// Values are normally created with Required or Optional.
type AcceptedParam interface {
	ParamDefinition
	paramName() string
	paramAvailable(callerpkg.Caller) bool
	paramPresent(any) bool
	parseParameter(parseContext, string, jsonObject) (parsedParam, *Error)
	parameterSpec() ParameterSpec
	wireType() Type
}

type paramSet interface {
	addAcceptedParam(AcceptedParam)
	addRule(rule)
}

// Parameter describes one accepted request parameter.
type Parameter[T any] struct {
	name         string
	required     bool
	shape        Shape[T]
	nullPolicy   NullPolicy
	availability callerpkg.Set
	parser       func(string, T) (any, *Error)
	minSize      *int64
	maxSize      *int64
	minItems     *int64
	maxItems     *int64
}

// Required declares a required request parameter with the provided JSON shape.
func Required[T any](name string, shape Shape[T]) *Parameter[T] {
	return newParameter(name, shape, true)
}

// Optional declares an optional request parameter with the provided JSON shape.
func Optional[T any](name string, shape Shape[T]) *Parameter[T] {
	return newParameter(name, shape, false)
}

func newParameter[T any](name string, shape Shape[T], required bool) *Parameter[T] {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("httpapi/param: parameter name is required")
	}
	if shape == nil {
		panic(fmt.Sprintf("httpapi/param: parameter %q shape is required", name))
	}
	return &Parameter[T]{
		name:       name,
		required:   required,
		shape:      shape,
		nullPolicy: NullAsAbsent,
	}
}

// Null sets how this parameter handles a present JSON null.
func (parameter *Parameter[T]) Null(policy NullPolicy) *Parameter[T] {
	parameter.nullPolicy = normalizeNullPolicy(policy)
	return parameter
}

// AvailableTo restricts which callers may supply this parameter.
func (parameter *Parameter[T]) AvailableTo(callers ...callerpkg.Caller) *Parameter[T] {
	parameter.availability = callerpkg.AvailableTo(callers...)
	return parameter
}

// MinSize sets the inclusive lower size bound for this parameter.
//
// The unit is type-dependent: strings use bytes, arrays use items, objects use
// fields, and numeric values use the numeric value.
func (parameter *Parameter[T]) MinSize(size int64) *Parameter[T] {
	if size < 0 {
		panic("httpapi/param: min size cannot be negative")
	}
	parameter.minSize = &size
	parameter.ensureBounds()
	return parameter
}

// MaxSize sets the inclusive upper size bound for this parameter.
func (parameter *Parameter[T]) MaxSize(size int64) *Parameter[T] {
	if size < 0 {
		panic("httpapi/param: max size cannot be negative")
	}
	parameter.maxSize = &size
	parameter.ensureBounds()
	return parameter
}

// MinItems sets the inclusive lower item count for an array parameter.
func (parameter *Parameter[T]) MinItems(items int64) *Parameter[T] {
	if items < 0 {
		panic("httpapi/param: min items cannot be negative")
	}
	if parameter.shape.wireType() != TypeArray {
		panic("httpapi/param: MinItems can only be used with array parameters")
	}
	parameter.minItems = &items
	parameter.ensureBounds()
	return parameter
}

// MaxItems sets the inclusive upper item count for an array parameter.
func (parameter *Parameter[T]) MaxItems(items int64) *Parameter[T] {
	if items < 0 {
		panic("httpapi/param: max items cannot be negative")
	}
	if parameter.shape.wireType() != TypeArray {
		panic("httpapi/param: MaxItems can only be used with array parameters")
	}
	parameter.maxItems = &items
	parameter.ensureBounds()
	return parameter
}

// Parse attaches domain parsing for this parameter.
//
// The parser must be a function with one input compatible with the parameter's
// shape type and two return values: the parsed domain value and an error.
// Parsers may also accept the parameter path as their first argument:
// func(path string, value T) (U, error). The parsed domain value may have a
// different type than the wire shape, which lets a string parameter parse into
// an ID type or an array of wire structs parse into domain line items.
func (parameter *Parameter[T]) Parse(parser any) *Parameter[T] {
	if parser == nil {
		panic("httpapi/param: parameter parser is required")
	}
	parameter.parser = buildParameterParser[T](parser)
	return parameter
}

func (parameter *Parameter[T]) addToParamSet(set paramSet) {
	if parameter == nil {
		panic("httpapi/param: parameter is required")
	}
	set.addAcceptedParam(parameter)
}

func (parameter *Parameter[T]) paramName() string {
	return parameter.name
}

func (parameter *Parameter[T]) paramAvailable(caller callerpkg.Caller) bool {
	return availabilityAllows(parameter.availability, caller)
}

func (parameter *Parameter[T]) paramPresent(raw any) bool {
	if raw != nil {
		return true
	}
	return normalizeNullPolicy(parameter.nullPolicy) != NullAsAbsent
}

func (parameter *Parameter[T]) wireType() Type {
	return parameter.shape.wireType()
}

func (parameter *Parameter[T]) parseParameter(
	ctx parseContext,
	parentPath string,
	object jsonObject,
) (parsedParam, *Error) {
	path := joinPath(parentPath, parameter.name)
	raw, ok := object[parameter.name]
	if !ok {
		if parameter.required {
			return parsedParam{}, missingError(path)
		}
		return parsedParam{}, nil
	}

	if raw == nil {
		switch normalizeNullPolicy(parameter.nullPolicy) {
		case NullAsAbsent:
			if parameter.required {
				return parsedParam{}, missingError(path)
			}
			return parsedParam{}, nil
		case NullRejected:
			return parsedParam{}, paramError(
				path,
				CodeNullRejected,
				fmt.Sprintf("`%s` must not be null", path),
				nil,
			)
		case NullAccepted:
			value := zeroValue[T]()
			return parameter.applyParser(path, value)
		}
	}

	if parameter.required && parameter.shape.wireType() == TypeString {
		if rawString, ok := raw.(string); ok && strings.TrimSpace(rawString) == "" {
			return parsedParam{}, missingError(path)
		}
	}

	if err := parameter.enforceSize(path, raw); err != nil {
		return parsedParam{}, err
	}

	value, err := parameter.shape.parseShape(ctx, path, raw)
	if err != nil {
		return parsedParam{}, err
	}
	return parameter.applyParser(path, value)
}

func (parameter *Parameter[T]) applyParser(path string, value T) (parsedParam, *Error) {
	if parameter.parser != nil {
		parsed, err := parameter.parser(path, value)
		if err != nil {
			return parsedParam{}, err
		}
		return parsedParam{value: parsed, present: true}, nil
	}
	return parsedParam{value: value, present: true}, nil
}

func (parameter *Parameter[T]) enforceSize(path string, raw any) *Error {
	if parameter.minSize != nil || parameter.maxSize != nil {
		size, unit, ok := parameter.measureSize(raw)
		if ok {
			if parameter.minSize != nil && size < float64(*parameter.minSize) {
				return paramError(
					path,
					CodeTooSmall,
					fmt.Sprintf("`%s` must be at least %s", path, describeSize(*parameter.minSize, unit)),
					nil,
				)
			}
			if parameter.maxSize != nil && size > float64(*parameter.maxSize) {
				return paramError(
					path,
					CodeTooLarge,
					fmt.Sprintf("`%s` must be at most %s", path, describeSize(*parameter.maxSize, unit)),
					nil,
				)
			}
		}
	}

	if parameter.minItems != nil || parameter.maxItems != nil {
		items, ok := raw.([]any)
		if !ok {
			return nil
		}
		size := float64(len(items))
		if parameter.minItems != nil && size < float64(*parameter.minItems) {
			return paramError(
				path,
				CodeTooSmall,
				fmt.Sprintf("`%s` must contain at least %s", path, describeSize(*parameter.minItems, "item")),
				nil,
			)
		}
		if parameter.maxItems != nil && size > float64(*parameter.maxItems) {
			return paramError(
				path,
				CodeTooLarge,
				fmt.Sprintf("`%s` must contain at most %s", path, describeSize(*parameter.maxItems, "item")),
				nil,
			)
		}
	}
	return nil
}

func (parameter *Parameter[T]) measureSize(raw any) (float64, string, bool) {
	switch parameter.shape.wireType() {
	case TypeString:
		value, ok := raw.(string)
		return float64(len(value)), "byte", ok
	case TypeInt:
		value, ok := rawInt(raw)
		return float64(value), "value", ok
	case TypeInt64:
		value, ok := rawInt64(raw)
		return float64(value), "value", ok
	case TypeFloat64:
		value, ok := rawFloat64(raw)
		return value, "value", ok
	case TypeArray:
		value, ok := raw.([]any)
		return float64(len(value)), "item", ok
	case TypeObject:
		value, ok := raw.(map[string]any)
		return float64(len(value)), "field", ok
	case TypeAny:
		return measuredSize(raw)
	default:
		return 0, "", false
	}
}

func (parameter *Parameter[T]) ensureBounds() {
	if parameter.minSize != nil && parameter.maxSize != nil &&
		*parameter.minSize > *parameter.maxSize {
		panic("httpapi/param: min size cannot exceed max size")
	}
	if parameter.minItems != nil && parameter.maxItems != nil &&
		*parameter.minItems > *parameter.maxItems {
		panic("httpapi/param: min items cannot exceed max items")
	}
}

func buildParameterParser[T any](parser any) func(string, T) (any, *Error) {
	value := reflect.ValueOf(parser)
	typ := value.Type()
	if typ.Kind() != reflect.Func {
		panic("httpapi/param: parameter parser must be a function")
	}
	if (typ.NumIn() != 1 && typ.NumIn() != 2) || typ.NumOut() != 2 {
		panic("httpapi/param: parameter parser must have signature func(T) (U, error) or func(string, T) (U, error)")
	}

	inputType := reflect.TypeFor[T]()
	inputArg := 0
	if typ.NumIn() == 2 {
		if typ.In(0) != reflect.TypeFor[string]() {
			panic("httpapi/param: path-aware parameter parser first input must be string")
		}
		inputArg = 1
	}
	if !inputType.AssignableTo(typ.In(inputArg)) {
		panic(fmt.Sprintf(
			"httpapi/param: parameter parser input %s is not compatible with %s",
			typ.In(inputArg),
			inputType,
		))
	}

	errorType := reflect.TypeFor[error]()
	if !typ.Out(1).Implements(errorType) {
		panic("httpapi/param: parameter parser second return value must implement error")
	}

	return func(path string, input T) (any, *Error) {
		arg := reflect.ValueOf(input)
		if !arg.IsValid() {
			arg = reflect.Zero(typ.In(inputArg))
		} else if !arg.Type().AssignableTo(typ.In(inputArg)) {
			return nil, parserError(path, fmt.Errorf(
				"cannot pass %s to parser expecting %s",
				arg.Type(),
				typ.In(inputArg),
			))
		}

		args := []reflect.Value{arg}
		if typ.NumIn() == 2 {
			args = []reflect.Value{reflect.ValueOf(path), arg}
		}
		output := value.Call(args)
		if !isNilReflectValue(output[1]) {
			return nil, parserError(path, output[1].Interface().(error))
		}
		return output[0].Interface(), nil
	}
}

func isNilReflectValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func missingError(path string) *Error {
	return paramError(
		path,
		CodeMissing,
		fmt.Sprintf("`%s` is required", path),
		nil,
	)
}
