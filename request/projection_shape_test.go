package request

import (
	"strings"
	"testing"

	callerpkg "github.com/zebodotdev/httpapi/caller"
	"github.com/zebodotdev/httpapi/param"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

var (
	projectionPublicCaller = callerpkg.Define("projection-public")
	projectionWorkerCaller = callerpkg.Define("projection-worker")
)

func TestDeriveResponseProjectionSchemaIndexesNestedTypedShapes(t *testing.T) {
	shape := responsepkg.ShapeSpec{
		Type: responsepkg.TypeObject,
		Attributes: []responsepkg.AttributeSpec{
			{
				Name: "purchase_intent",
				Shape: responsepkg.ShapeSpec{
					Type: responsepkg.TypeObject,
					Attributes: []responsepkg.AttributeSpec{
						{
							Name: "product",
							Shape: responsepkg.ShapeSpec{
								Type: responsepkg.TypeObject,
								Attributes: []responsepkg.AttributeSpec{
									{Name: "id", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeString}},
									{
										Name: "media",
										Shape: responsepkg.ShapeSpec{
											Type: responsepkg.TypeObject,
											Attributes: []responsepkg.AttributeSpec{
												{Name: "gallery", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeArray}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	schema := DeriveResponseProjectionSchema(shape)
	productRoots := schema.resourceRoots("product")
	mediaRoots := schema.resourceRoots("media")
	if len(productRoots) != 1 || len(mediaRoots) != 1 {
		t.Fatalf("resource roots: product=%v media=%v", productRoots, mediaRoots)
	}
	mediaChild, ok := schema.child(productRoots[0], "media")
	if !ok || mediaChild != mediaRoots[0] {
		t.Fatalf("nested media node = %d, resource root = %v", mediaChild, mediaRoots)
	}
	for _, edge := range schema.edges {
		if strings.Contains(edge.segment, ".") {
			t.Fatalf("trie edge stores dotted path %q", edge.segment)
		}
	}

	projection := mustResponseProjection(t, map[string]PropertyProjection{
		"product": mustPickProperties(t, "id", "media.gallery"),
		"media":   mustOmitProperties(t, "gallery"),
	})
	if err := schema.Validate(projection, projectionPublicCaller); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestDerivedResponseProjectionSchemaHonorsCallerAvailability(t *testing.T) {
	shape := responsepkg.ShapeSpec{
		Type: responsepkg.TypeObject,
		Attributes: []responsepkg.AttributeSpec{
			{
				Name: "product",
				Shape: responsepkg.ShapeSpec{
					Type: responsepkg.TypeObject,
					Attributes: []responsepkg.AttributeSpec{
						{Name: "id", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeString}},
						{
							Name:         "internal_note",
							Availability: callerpkg.AvailableTo(projectionWorkerCaller),
							Shape:        responsepkg.ShapeSpec{Type: responsepkg.TypeString},
						},
					},
				},
			},
		},
	}
	schema := DeriveResponseProjectionSchema(shape)
	projection := mustResponseProjection(t, map[string]PropertyProjection{
		"product": mustPickProperties(t, "internal_note"),
	})

	publicErr := schema.Validate(projection, projectionPublicCaller)
	if publicErr == nil || publicErr.Code != param.CodeInvalidResponseProjection {
		t.Fatalf("public error = %#v", publicErr)
	}
	if err := schema.Validate(projection, projectionWorkerCaller); err != nil {
		t.Fatalf("worker error = %v", err)
	}
}

func TestDerivedResponseProjectionSchemaRejectsUnknownResource(t *testing.T) {
	shape := responsepkg.ShapeSpec{
		Type: responsepkg.TypeObject,
		Attributes: []responsepkg.AttributeSpec{
			{
				Name: "product",
				Shape: responsepkg.ShapeSpec{
					Type: responsepkg.TypeObject,
					Attributes: []responsepkg.AttributeSpec{
						{Name: "id", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeString}},
					},
				},
			},
		},
	}
	schema := DeriveResponseProjectionSchema(shape)
	projection := mustResponseProjection(t, map[string]PropertyProjection{
		"products": mustPickProperties(t, "id"),
	})

	err := schema.Validate(projection, projectionPublicCaller)
	if err == nil || err.Code != param.CodeInvalidResponseProjection {
		t.Fatalf("error = %#v", err)
	}
	if err.Param != "request_meta.response.projection.products" {
		t.Fatalf("error param = %q", err.Param)
	}
}

func TestDerivedResponseProjectionSchemaTraversesArrayAndMapValues(t *testing.T) {
	item := responsepkg.ShapeSpec{
		Type: responsepkg.TypeObject,
		Attributes: []responsepkg.AttributeSpec{
			{Name: "id", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeString}},
		},
	}
	shape := responsepkg.ShapeSpec{
		Type: responsepkg.TypeObject,
		Attributes: []responsepkg.AttributeSpec{
			{
				Name:  "products",
				Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeArray, Item: &item},
			},
			{
				Name:  "countries",
				Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeObject, MapValue: &item},
			},
		},
	}
	schema := DeriveResponseProjectionSchema(shape)
	projection := mustResponseProjection(t, map[string]PropertyProjection{
		"products":  mustPickProperties(t, "id"),
		"countries": mustPickProperties(t, "id"),
	})

	if err := schema.Validate(projection, projectionPublicCaller); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func mustPickProperties(t *testing.T, paths ...string) PropertyProjection {
	t.Helper()
	projection, err := PickProperties(paths...)
	if err != nil {
		t.Fatalf("PickProperties error = %v", err)
	}
	return projection
}

func mustOmitProperties(t *testing.T, paths ...string) PropertyProjection {
	t.Helper()
	projection, err := OmitProperties(paths...)
	if err != nil {
		t.Fatalf("OmitProperties error = %v", err)
	}
	return projection
}

func mustResponseProjection(
	t *testing.T,
	resources map[string]PropertyProjection,
) ResponseProjection {
	t.Helper()
	projection, err := DefineResponseProjection(resources)
	if err != nil {
		t.Fatalf("DefineResponseProjection error = %v", err)
	}
	return projection
}
