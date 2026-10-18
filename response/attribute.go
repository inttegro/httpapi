package response

import (
	"fmt"
	"strings"

	callerpkg "github.com/zebodotdev/httpapi/caller"
)

// AcceptedAttribute is a single response attribute that can be added to an
// ObjectShape.
//
// Values are normally created with Required or Optional. The interface is kept
// small so object shapes can accept typed attributes without exposing their
// internal getter or projection machinery.
type AcceptedAttribute[T any] interface {
	attributeName() string
	attributeSpec() AttributeSpec
	projectAttribute(projectionContext, T) (string, any, bool)
}

// AttributeSpec is the transcribable description of a response object
// attribute.
//
// Spec writers should use Availability to decide whether an attribute belongs in
// a public, internal, or caller-specific document. Runtime rendering uses the
// same availability to omit attributes from shaped response bodies.
type AttributeSpec struct {
	// Name is the JSON key emitted for the attribute.
	Name string

	// Required reports whether the attribute is always emitted when its caller
	// availability allows it.
	Required bool

	// Availability is the caller allow-list for the attribute. The zero value is
	// unrestricted.
	Availability callerpkg.Set

	// Shape describes the JSON value emitted for this attribute.
	Shape ShapeSpec
}

// Attribute describes one response attribute.
//
// Attribute is intentionally parameterized by both the source view type T and
// the emitted value type V. This lets a handler return a domain-facing view
// struct while each attribute getter extracts exactly the JSON value that should
// be exposed.
type Attribute[T any, V any] struct {
	name         string
	required     bool
	shape        Shape[V]
	availability callerpkg.Set
	getter       func(T) (V, bool)
}

// Required declares a response attribute that is emitted whenever its caller
// availability allows it.
//
// Required attributes use a getter that always returns a value. They are still
// omitted for callers excluded by AvailableTo.
func Required[T any, V any](
	name string,
	shape Shape[V],
	getter func(T) V,
) *Attribute[T, V] {
	if getter == nil {
		panic("httpapi/response: required attribute getter is required")
	}
	attribute := newAttribute(name, shape, func(source T) (V, bool) {
		return getter(source), true
	})
	attribute.required = true
	return attribute
}

// Optional declares a response attribute that may be omitted by its getter.
//
// The getter's bool return reports whether the attribute is present. Returning
// false omits the attribute instead of rendering a zero value or null.
func Optional[T any, V any](
	name string,
	shape Shape[V],
	getter func(T) (V, bool),
) *Attribute[T, V] {
	if getter == nil {
		panic("httpapi/response: optional attribute getter is required")
	}
	return newAttribute(name, shape, getter)
}

func newAttribute[T any, V any](
	name string,
	shape Shape[V],
	getter func(T) (V, bool),
) *Attribute[T, V] {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("httpapi/response: attribute name is required")
	}
	if shape == nil {
		panic(fmt.Sprintf("httpapi/response: attribute %q shape is required", name))
	}
	return &Attribute[T, V]{
		name:   name,
		shape:  shape,
		getter: getter,
	}
}

// AvailableTo restricts which callers may see this attribute.
//
// Omit AvailableTo to expose the attribute to every caller. When a caller is not
// allowed, the attribute is omitted from runtime responses and can be omitted
// from caller-filtered generated specs.
func (attribute *Attribute[T, V]) AvailableTo(callers ...callerpkg.Caller) *Attribute[T, V] {
	attribute.availability = callerpkg.AvailableTo(callers...)
	return attribute
}

func (attribute *Attribute[T, V]) attributeName() string {
	return attribute.name
}

func (attribute *Attribute[T, V]) attributeSpec() AttributeSpec {
	return AttributeSpec{
		Name:         attribute.name,
		Required:     attribute.required,
		Availability: cloneCallerSet(attribute.availability),
		Shape:        attribute.shape.describeShape(),
	}
}

func (attribute *Attribute[T, V]) projectAttribute(
	context projectionContext,
	source T,
) (string, any, bool) {
	if !attribute.availability.Allows(context.Caller) || !context.includesAttribute(attribute.name) {
		return "", nil, false
	}

	value, ok := attribute.getter(source)
	if !ok {
		return "", nil, false
	}
	return attribute.name, attribute.shape.projectShape(context.descend(attribute.name), value), true
}

func cloneCallerSet(set callerpkg.Set) callerpkg.Set {
	if !set.Restricted() {
		return callerpkg.Set{}
	}
	return callerpkg.SetOf(set.Callers()...)
}
