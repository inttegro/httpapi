package response

import (
	"fmt"
	"reflect"
	"strings"

	callerpkg "github.com/zebodotdev/httpapi/caller"
)

// EnvelopeShape defines a top-level JSON object response shape from named
// attributes, without requiring an endpoint-local envelope struct.
//
// Use Envelope when a handler only needs to wrap one or more already-shaped
// values under stable JSON names, such as {"task": ...} or {"error": ...}.
// For domain views that need computed attributes, Object and Project remain the
// better fit.
type EnvelopeShape struct {
	attributes []AcceptedEnvelopeAttribute
}

// Envelope starts a JSON envelope response shape definition.
//
// Define envelopes once, usually at package level beside the endpoint response
// shapes they wrap, then render them with EnvelopeShape.Body and Fields.
func Envelope(attributes ...AcceptedEnvelopeAttribute) *EnvelopeShape {
	if len(attributes) == 0 {
		panic("httpapi/response: envelope requires at least one attribute")
	}
	shape := &EnvelopeShape{}
	for _, attribute := range attributes {
		shape.Attribute(attribute)
	}
	return shape
}

// Attribute adds a response attribute to this envelope shape.
func (shape *EnvelopeShape) Attribute(attribute AcceptedEnvelopeAttribute) *EnvelopeShape {
	if shape == nil {
		panic("httpapi/response: nil envelope shape")
	}
	if attribute == nil {
		panic("httpapi/response: envelope attribute is required")
	}
	for _, existing := range shape.attributes {
		if existing.attributeName() == attribute.attributeName() {
			panic(fmt.Sprintf("httpapi/response: duplicate envelope attribute %q", attribute.attributeName()))
		}
		if existing.attributeType() == attribute.attributeType() {
			panic(fmt.Sprintf(
				"httpapi/response: envelope attributes %q and %q share Go type %s",
				existing.attributeName(),
				attribute.attributeName(),
				attribute.attributeType(),
			))
		}
	}
	shape.attributes = append(shape.attributes, attribute)
	return shape
}

// ProjectForCaller returns the JSON envelope visible to caller.
//
// An envelope response must emit at least one non-nil field for the active
// caller. Optional nil fields are omitted; required nil fields panic.
func (shape *EnvelopeShape) ProjectForCaller(values EnvelopeValues, caller callerpkg.Caller) map[string]any {
	if shape == nil {
		panic("httpapi/response: nil envelope shape")
	}
	shape.validateAttributes()
	shape.validateValues(values)

	object := make(map[string]any, len(shape.attributes))
	for _, attribute := range shape.attributes {
		name, projected, ok := attribute.projectEnvelopeAttribute(caller, values)
		if ok {
			object[name] = projected
		}
	}
	if len(object) == 0 {
		panic("httpapi/response: envelope projected no fields")
	}
	return object
}

// Attributes returns this envelope shape's transcribable attribute descriptions.
func (shape *EnvelopeShape) Attributes() []AttributeSpec {
	if shape == nil {
		panic("httpapi/response: nil envelope shape")
	}
	shape.validateAttributes()

	attributes := make([]AttributeSpec, 0, len(shape.attributes))
	for _, attribute := range shape.attributes {
		attributes = append(attributes, attribute.attributeSpec())
	}
	return attributes
}

// Body returns a caller-aware body from named field values.
//
// Body is the usual handler-facing entrypoint:
//
//	response.RenderJSON(r, http.StatusOK, taskEnvelope.Body(
//		response.Field("task", task),
//	))
func (shape *EnvelopeShape) Body(fields ...EnvelopeField) ShapedBody[EnvelopeValues] {
	return shape.BodyValues(Fields(fields...))
}

// BodyValues returns a caller-aware body from a prebuilt field collection.
//
// Prefer Body for simple endpoint responses. BodyValues is useful when a helper
// has already assembled an EnvelopeValues value.
func (shape *EnvelopeShape) BodyValues(values EnvelopeValues) ShapedBody[EnvelopeValues] {
	return Body[EnvelopeValues](shape, values)
}

func (shape *EnvelopeShape) describeShape() ShapeSpec {
	return ShapeSpec{Type: TypeObject, Attributes: shape.Attributes()}
}

func (shape *EnvelopeShape) projectShape(caller callerpkg.Caller, values EnvelopeValues) any {
	return shape.ProjectForCaller(values, caller)
}

func (shape *EnvelopeShape) validateAttributes() {
	if len(shape.attributes) == 0 {
		panic("httpapi/response: envelope requires at least one attribute")
	}

	names := make(map[string]struct{}, len(shape.attributes))
	types := make(map[reflect.Type]string, len(shape.attributes))
	for _, attribute := range shape.attributes {
		name := attribute.attributeName()
		if _, ok := names[name]; ok {
			panic(fmt.Sprintf("httpapi/response: duplicate envelope attribute %q", name))
		}
		names[name] = struct{}{}

		typ := attribute.attributeType()
		if existing, ok := types[typ]; ok {
			panic(fmt.Sprintf(
				"httpapi/response: envelope attributes %q and %q share Go type %s",
				existing,
				name,
				typ,
			))
		}
		types[typ] = name
	}
}

func (shape *EnvelopeShape) validateValues(values EnvelopeValues) {
	if values.values == nil {
		return
	}

	names := make(map[string]struct{}, len(shape.attributes))
	for _, attribute := range shape.attributes {
		names[attribute.attributeName()] = struct{}{}
	}
	for name := range values.values {
		if _, ok := names[name]; !ok {
			panic(fmt.Sprintf("httpapi/response: unexpected envelope field %q", name))
		}
	}
}

// AcceptedEnvelopeAttribute is a single top-level attribute accepted by an
// EnvelopeShape.
type AcceptedEnvelopeAttribute interface {
	attributeName() string
	attributeType() reflect.Type
	attributeSpec() AttributeSpec
	projectEnvelopeAttribute(callerpkg.Caller, EnvelopeValues) (string, any, bool)
}

// EnvelopeAttribute describes one top-level envelope attribute.
type EnvelopeAttribute[T any] struct {
	name         string
	valueType    reflect.Type
	required     bool
	shape        Shape[T]
	availability callerpkg.Set
}

// RequiredField declares an envelope attribute that must be provided in Fields.
//
// RequiredField is per-field contract metadata. It is not needed merely to make
// the envelope non-empty; every envelope projection must emit at least one
// non-nil field regardless of whether its accepted fields are required or
// optional.
func RequiredField[T any](name string, shape Shape[T]) *EnvelopeAttribute[T] {
	attribute := newEnvelopeAttribute(name, shape)
	attribute.required = true
	return attribute
}

// OptionalField declares an envelope attribute that is omitted when Fields does
// not contain a value for its name or when its provided value is nil.
func OptionalField[T any](name string, shape Shape[T]) *EnvelopeAttribute[T] {
	return newEnvelopeAttribute(name, shape)
}

func newEnvelopeAttribute[T any](name string, shape Shape[T]) *EnvelopeAttribute[T] {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("httpapi/response: envelope attribute name is required")
	}
	if shape == nil {
		panic(fmt.Sprintf("httpapi/response: envelope attribute %q shape is required", name))
	}
	return &EnvelopeAttribute[T]{
		name:      name,
		valueType: envelopeTypeOf[T](),
		shape:     shape,
	}
}

// AvailableTo restricts which callers may see this envelope attribute.
func (attribute *EnvelopeAttribute[T]) AvailableTo(callers ...callerpkg.Caller) *EnvelopeAttribute[T] {
	attribute.availability = callerpkg.AvailableTo(callers...)
	return attribute
}

func (attribute *EnvelopeAttribute[T]) attributeName() string {
	return attribute.name
}

func (attribute *EnvelopeAttribute[T]) attributeType() reflect.Type {
	return attribute.valueType
}

func (attribute *EnvelopeAttribute[T]) attributeSpec() AttributeSpec {
	return AttributeSpec{
		Name:         attribute.name,
		Required:     attribute.required,
		Availability: cloneCallerSet(attribute.availability),
		Shape:        attribute.shape.describeShape(),
	}
}

func (attribute *EnvelopeAttribute[T]) projectEnvelopeAttribute(
	caller callerpkg.Caller,
	values EnvelopeValues,
) (string, any, bool) {
	if !attribute.availability.Allows(caller) {
		return "", nil, false
	}

	value, ok := values.value(attribute.name)
	if !ok {
		if attribute.required {
			panic(fmt.Sprintf("httpapi/response: required envelope field %q was not provided", attribute.name))
		}
		return "", nil, false
	}
	if envelopeValueNil(value) {
		if attribute.required {
			panic(fmt.Sprintf("httpapi/response: required envelope field %q was nil", attribute.name))
		}
		return "", nil, false
	}

	typed, ok := value.(T)
	if !ok {
		panic(fmt.Sprintf("httpapi/response: envelope field %q has type %T, want %T", attribute.name, value, *new(T)))
	}

	return attribute.name, attribute.shape.projectShape(caller, typed), true
}

func envelopeTypeOf[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}

func envelopeValueNil(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// EnvelopeValues contains the concrete values rendered by an EnvelopeShape.
type EnvelopeValues struct {
	values map[string]any
}

// Fields returns values for an EnvelopeShape.
func Fields(fields ...EnvelopeField) EnvelopeValues {
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		if field.name == "" {
			panic("httpapi/response: envelope field name is required")
		}
		if _, ok := values[field.name]; ok {
			panic(fmt.Sprintf("httpapi/response: duplicate envelope field %q", field.name))
		}
		values[field.name] = field.value
	}
	return EnvelopeValues{values: values}
}

func (values EnvelopeValues) value(name string) (any, bool) {
	if values.values == nil {
		return nil, false
	}
	value, ok := values.values[name]
	return value, ok
}

// EnvelopeField is one concrete envelope value.
type EnvelopeField struct {
	name  string
	value any
}

// Field provides one named value for an EnvelopeShape.
func Field[T any](name string, value T) EnvelopeField {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("httpapi/response: envelope field name is required")
	}
	return EnvelopeField{
		name:  name,
		value: value,
	}
}
