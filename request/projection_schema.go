package request

import (
	"fmt"
	"sort"
	"strings"

	callerpkg "github.com/zebodotdev/httpapi/caller"
	"github.com/zebodotdev/httpapi/param"
)

// ResponseProjectionSchema is the immutable, flat prefix tree derived from an
// endpoint's typed response contracts.
//
// Nodes and edges live in contiguous slices. Resource names point to one or
// more root-node indexes so repeated response resources can share their actual
// shape nodes without copying complete dotted property paths.
//
// This is intentionally a typed in-memory index rather than an opaque encoded
// byte string. uint32 indexes and sorted edge ranges provide the useful
// properties of a compact binary representation while keeping bounds checks,
// inspection, and evolution straightforward. Edge labels also reuse the
// property strings already present in response contracts; packing them into a
// second string table would add decoding and offset management without a
// demonstrated memory or latency benefit. If profiling eventually identifies
// labels as material, they can be interned without changing projection
// semantics or the validation API.
type ResponseProjectionSchema struct {
	resources []responseProjectionResource
	roots     []uint32
	nodes     []responseProjectionNode
	edges     []responseProjectionEdge
}

// responseProjectionResource describes a contiguous range in roots. A resource
// may have multiple roots because the same resource name can occur in more than
// one successful response contract with a different shape or availability.
type responseProjectionResource struct {
	name      string
	firstRoot uint32
	rootCount uint32
}

// responseProjectionNode stores only caller visibility and a range into edges.
// The property name belongs to the incoming edge, allowing a node to be shared
// by both a nested property and a top-level resource root.
type responseProjectionNode struct {
	availability callerpkg.Set
	firstEdge    uint32
	edgeCount    uint32
}

type responseProjectionEdge struct {
	segment string
	child   uint32
}

// ValidateForCaller validates projection against the resources and properties
// visible to caller. It prevents a projection error from revealing restricted
// response attributes to callers that cannot receive them.
func (schema ResponseProjectionSchema) ValidateForCaller(
	projection ResponseProjection,
	caller callerpkg.Caller,
) *param.Error {
	validator := responseProjectionValidator{
		schema:     schema,
		projection: projection,
		caller:     caller,
	}
	validator.validateResources()
	return validator.err
}

type responseProjectionValidator struct {
	schema     ResponseProjectionSchema
	projection ResponseProjection
	caller     callerpkg.Caller
	err        *param.Error
}

func (validator *responseProjectionValidator) validateResources() {
	if validator.err != nil || !validator.projection.Defined() {
		return
	}
	for _, resource := range validator.projection.Resources() {
		validator.validateResource(resource)
		if validator.err != nil {
			return
		}
	}
}

func (validator *responseProjectionValidator) validateResource(resource string) {
	roots := validator.schema.resourceRoots(resource)
	if !validator.hasVisibleRoot(roots) {
		validator.err = invalidResponseProjection(
			joinMetaPath(projectionPath(), resource),
			fmt.Sprintf("`%s` is not a projectable response resource", resource),
		)
		return
	}

	selection, _ := validator.projection.Resource(resource)
	for index, path := range selection.Paths() {
		if validator.pathAvailable(roots, path) {
			continue
		}
		selector := joinMetaPath(
			joinMetaPath(projectionPath(), resource),
			string(selection.Mode()),
		)
		validator.err = invalidResponseProjection(
			fmt.Sprintf("%s[%d]", selector, index),
			fmt.Sprintf("`%s` is not a projectable property of `%s`", path, resource),
		)
		return
	}
}

func (schema ResponseProjectionSchema) resourceRoots(resource string) []uint32 {
	index := sort.Search(len(schema.resources), func(index int) bool {
		return schema.resources[index].name >= resource
	})
	if index >= len(schema.resources) || schema.resources[index].name != resource {
		return nil
	}
	entry := schema.resources[index]
	start := int(entry.firstRoot)
	end := start + int(entry.rootCount)
	if end > len(schema.roots) {
		return nil
	}
	return schema.roots[start:end]
}

func (validator *responseProjectionValidator) hasVisibleRoot(roots []uint32) bool {
	for _, root := range roots {
		if validator.schema.nodeVisibleTo(root, validator.caller) {
			return true
		}
	}
	return false
}

func (validator *responseProjectionValidator) pathAvailable(
	roots []uint32,
	path string,
) bool {
	for _, root := range roots {
		if validator.schema.pathAvailableTo(root, path, validator.caller) {
			return true
		}
	}
	return false
}

func (schema ResponseProjectionSchema) pathAvailableTo(
	root uint32,
	path string,
	caller callerpkg.Caller,
) bool {
	if !schema.nodeVisibleTo(root, caller) {
		return false
	}

	node := root
	remaining := path
	for remaining != "" {
		// Walk the caller's string a segment at a time instead of allocating a
		// []string with strings.Split on every validation.
		segment, rest, found := strings.Cut(remaining, ".")
		child, ok := schema.child(node, segment)
		if !ok || !schema.nodeVisibleTo(child, caller) {
			return false
		}
		node = child
		if !found {
			return true
		}
		remaining = rest
	}
	return false
}

func (schema ResponseProjectionSchema) child(
	nodeIndex uint32,
	segment string,
) (uint32, bool) {
	if int(nodeIndex) >= len(schema.nodes) {
		return 0, false
	}
	node := schema.nodes[nodeIndex]
	start := int(node.firstEdge)
	end := start + int(node.edgeCount)
	if end > len(schema.edges) {
		return 0, false
	}
	edges := schema.edges[start:end]
	// Edges are sorted once while compiling the immutable schema. Binary search
	// keeps validation allocation-free without retaining a map per trie node.
	index := sort.Search(len(edges), func(index int) bool {
		return edges[index].segment >= segment
	})
	if index >= len(edges) || edges[index].segment != segment {
		return 0, false
	}
	return edges[index].child, true
}

func (schema ResponseProjectionSchema) nodeVisibleTo(
	index uint32,
	caller callerpkg.Caller,
) bool {
	if int(index) >= len(schema.nodes) {
		return false
	}
	return schema.nodes[index].availability.Allows(caller)
}

func invalidResponseProjection(path string, message string) *param.Error {
	return param.NewError(
		path,
		param.CodeInvalidResponseProjection,
		message,
		nil,
	)
}
