package response

import (
	"strings"

	callerpkg "github.com/zebodotdev/httpapi/caller"
)

// Projection is the response-property selection supplied by a caller.
//
// request.ResponseProjection satisfies this interface. Keeping the response
// package coupled to this small behavior rather than the request package avoids
// an import cycle while letting typed shapes apply a validated projection as
// they extract attributes.
type Projection interface {
	// Defined reports whether the caller selected any response resources.
	Defined() bool

	// Includes reports whether path remains selected for resource. Resources
	// omitted from the request projection include every property.
	Includes(resource string, path string) bool
}

// Context carries the authority and property selection used to render a
// response. Rendering context is data rather than a collection of specialized
// entry points; adding another rendering concern should extend this value, not
// lengthen function names.
type Context struct {
	Caller     callerpkg.Caller
	Projection Projection
}

// ContextTarget exposes the context used to render a response for a target.
//
// request.Req implements this interface. Custom response targets may implement
// it when they need the same behavior outside the standard endpoint runtime.
type ContextTarget interface {
	Target

	ResponseContext() Context
}

type projectionContext struct {
	Context
	resources []projectionResource
}

type projectionResource struct {
	name string
	path string
}

func newProjectionContext(context Context) projectionContext {
	if context.Projection == nil || !context.Projection.Defined() {
		context.Projection = nil
	}
	return projectionContext{
		Context: context,
	}
}

// includesAttribute is evaluated before the attribute getter. This is the
// performance boundary of response projection: omitted attributes are never
// extracted, prepared, recursively shaped, or added to the intermediate JSON
// object merely to be discarded before encoding.
func (context projectionContext) includesAttribute(name string) bool {
	if context.Projection == nil {
		return true
	}
	for _, resource := range context.resources {
		if !context.Projection.Includes(resource.name, joinPropertyPath(resource.path, name)) {
			return false
		}
	}
	return true
}

// descend keeps every enclosing resource path active and starts a new resource
// scope at the named attribute. The projection schema uses the same rule when
// indexing nested typed objects, so a projection rooted at "product" applies
// to every product object while a projection rooted at "purchase_intent" can
// still constrain product through "product.*". When both are supplied their
// selections intersect, which prevents either scope from widening the other.
func (context projectionContext) descend(name string) projectionContext {
	if context.Projection == nil {
		return context
	}
	resources := make([]projectionResource, 0, len(context.resources)+1)
	for _, resource := range context.resources {
		resources = append(resources, projectionResource{
			name: resource.name,
			path: joinPropertyPath(resource.path, name),
		})
	}
	resources = append(resources, projectionResource{name: name})
	context.resources = resources
	return context
}

func joinPropertyPath(parent string, child string) string {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return parent + "." + child
}
