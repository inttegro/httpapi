package request

import (
	"sort"

	callerpkg "github.com/inttegro/httpapi/caller"
	responsepkg "github.com/inttegro/httpapi/response"
)

// DeriveResponseProjectionSchema compiles typed response shapes into a flat
// prefix tree.
//
// Response contracts are the source of truth for both serialization and
// projection. Deriving the index from them avoids a second, manually maintained
// projection schema that could drift from the endpoint's real response. The
// endpoint runtime calls this function lazily on the first request that
// actually supplies a projection.
func DeriveResponseProjectionSchema(
	shapes ...responsepkg.ShapeSpec,
) ResponseProjectionSchema {
	builder := responseProjectionTreeBuilder{
		resources: make(map[string][]*responseProjectionTreeNode),
	}
	for _, shape := range shapes {
		builder.indexResources(shape, callerpkg.Set{})
	}
	return builder.compile()
}

type responseProjectionTreeNode struct {
	availability callerpkg.Set
	children     map[string]*responseProjectionTreeNode
}

// responseProjectionTreeBuilder favors simple maps while discovering the graph.
// It exists only
// during the one-time compilation; compile replaces it with the immutable flat
// representation, so request-time memory does not retain a map for every node.
type responseProjectionTreeBuilder struct {
	resources map[string][]*responseProjectionTreeNode
}

func (builder *responseProjectionTreeBuilder) indexResources(
	shape responsepkg.ShapeSpec,
	availability callerpkg.Set,
) {
	shape = projectionObjectShape(shape)
	if shape.Type != responsepkg.TypeObject {
		return
	}

	for _, attribute := range shape.Attributes {
		attributeAvailability := availability.Intersect(attribute.Availability)
		projectable := projectionObjectShape(attribute.Shape)
		if projectable.Type != responsepkg.TypeObject {
			continue
		}

		node := builder.buildNode(projectable, attributeAvailability)
		builder.resources[attribute.Name] = append(builder.resources[attribute.Name], node)
	}
}

func (builder *responseProjectionTreeBuilder) buildNode(
	shape responsepkg.ShapeSpec,
	availability callerpkg.Set,
) *responseProjectionTreeNode {
	node := &responseProjectionTreeNode{
		availability: availability,
		children:     make(map[string]*responseProjectionTreeNode, len(shape.Attributes)),
	}
	for _, attribute := range shape.Attributes {
		attributeAvailability := availability.Intersect(attribute.Availability)
		childShape := projectionObjectShape(attribute.Shape)
		child := &responseProjectionTreeNode{availability: attributeAvailability}
		if childShape.Type == responsepkg.TypeObject {
			child = builder.buildNode(childShape, attributeAvailability)
			// Register the actual child node as a resource root. Reusing this
			// pointer lets the compiled graph share one subtree for both
			// "product.media" and a projection rooted at "media".
			builder.resources[attribute.Name] = append(builder.resources[attribute.Name], child)
		}
		node.children[attribute.Name] = child
	}
	return node
}

func (builder *responseProjectionTreeBuilder) compile() ResponseProjectionSchema {
	compiler := responseProjectionTreeCompiler{
		resources: builder.resources,
		indexes:   make(map[*responseProjectionTreeNode]uint32),
	}
	compiler.compileResources()
	return compiler.schema
}

type responseProjectionTreeCompiler struct {
	resources map[string][]*responseProjectionTreeNode
	indexes   map[*responseProjectionTreeNode]uint32
	schema    ResponseProjectionSchema
}

func (compiler *responseProjectionTreeCompiler) compileResources() {
	resources := make([]string, 0, len(compiler.resources))
	for resource := range compiler.resources {
		resources = append(resources, resource)
	}
	sort.Strings(resources)

	for _, resource := range resources {
		roots := compiler.resources[resource]
		firstRoot := uint32(len(compiler.schema.roots))
		for _, root := range roots {
			compiler.schema.roots = append(
				compiler.schema.roots,
				compiler.compileNode(root),
			)
		}
		compiler.schema.resources = append(
			compiler.schema.resources,
			responseProjectionResource{
				name:      resource,
				firstRoot: firstRoot,
				rootCount: uint32(len(roots)),
			},
		)
	}
}

func (compiler *responseProjectionTreeCompiler) compileNode(
	node *responseProjectionTreeNode,
) uint32 {
	// Pointer identity is deliberate: a nested object can also be registered as
	// a named resource root. Compiling it once preserves prefix sharing rather
	// than duplicating its complete subtree.
	if index, exists := compiler.indexes[node]; exists {
		return index
	}

	index := uint32(len(compiler.schema.nodes))
	compiler.indexes[node] = index
	compiler.schema.nodes = append(compiler.schema.nodes, responseProjectionNode{
		availability: node.availability,
	})

	names := make([]string, 0, len(node.children))
	for name := range node.children {
		names = append(names, name)
	}
	sort.Strings(names)

	firstEdge := uint32(len(compiler.schema.edges))
	// Reserve this node's complete edge range before recursively compiling
	// children. Recursion appends other edge ranges, so writing into preassigned
	// slots is what keeps every node's edges contiguous and sorted.
	compiler.schema.edges = append(
		compiler.schema.edges,
		make([]responseProjectionEdge, len(names))...,
	)
	for edgeIndex, name := range names {
		child := compiler.compileNode(node.children[name])
		compiler.schema.edges[int(firstEdge)+edgeIndex] = responseProjectionEdge{
			segment: name,
			child:   child,
		}
	}
	compiler.schema.nodes[index].firstEdge = firstEdge
	compiler.schema.nodes[index].edgeCount = uint32(len(names))
	return index
}

func projectionObjectShape(shape responsepkg.ShapeSpec) responsepkg.ShapeSpec {
	// Projection names resources, not transport containers. Arrays project their
	// item object and maps project their value object, matching the shape callers
	// actually observe for each resource instance.
	for {
		switch shape.Type {
		case responsepkg.TypeArray:
			if shape.Item == nil {
				return responsepkg.ShapeSpec{}
			}
			shape = *shape.Item
		case responsepkg.TypeObject:
			if len(shape.Attributes) > 0 || shape.MapValue == nil {
				return shape
			}
			shape = *shape.MapValue
		default:
			return shape
		}
	}
}
