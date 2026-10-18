package request

import (
	"fmt"
	"testing"

	"github.com/zebodotdev/httpapi/param"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

var benchmarkProjectionError *param.Error

func BenchmarkResponseProjectionValidation(b *testing.B) {
	benchmarks := []struct {
		name          string
		resourceCount int
		propertyCount int
		selectedPaths []string
	}{
		{
			name:          "typical",
			resourceCount: 4,
			propertyCount: 24,
			selectedPaths: []string{"property_00", "property_11", "details.gallery.url"},
		},
		{
			name:          "large_valid",
			resourceCount: 64,
			propertyCount: 64,
			selectedPaths: []string{
				"property_00",
				"property_07",
				"property_15",
				"property_31",
				"property_47",
				"property_63",
				"details.gallery.url",
				"details.gallery.alt",
			},
		},
		{
			name:          "large_invalid",
			resourceCount: 64,
			propertyCount: 64,
			selectedPaths: []string{"details.gallery.missing"},
		},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			shape := benchmarkProjectionShape(
				benchmark.resourceCount,
				benchmark.propertyCount,
			)
			schema := DeriveResponseProjectionSchema(shape)
			selection, err := PickProperties(benchmark.selectedPaths...)
			if err != nil {
				b.Fatal(err)
			}
			projection, err := DefineResponseProjection(map[string]PropertyProjection{
				fmt.Sprintf("resource_%02d", benchmark.resourceCount-1): selection,
			})
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				benchmarkProjectionError = schema.ValidateForCaller(
					projection,
					projectionPublicCaller,
				)
			}
		})
	}
}

func BenchmarkDeriveResponseProjectionSchema(b *testing.B) {
	benchmarks := []struct {
		name          string
		resourceCount int
		propertyCount int
	}{
		{name: "typical", resourceCount: 4, propertyCount: 24},
		{name: "large", resourceCount: 64, propertyCount: 64},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			shape := benchmarkProjectionShape(
				benchmark.resourceCount,
				benchmark.propertyCount,
			)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = DeriveResponseProjectionSchema(shape)
			}
		})
	}
}

func benchmarkProjectionShape(resourceCount int, propertyCount int) responsepkg.ShapeSpec {
	resources := make([]responsepkg.AttributeSpec, 0, resourceCount)
	for resourceIndex := range resourceCount {
		properties := make([]responsepkg.AttributeSpec, 0, propertyCount+1)
		for propertyIndex := range propertyCount {
			properties = append(properties, responsepkg.AttributeSpec{
				Name: fmt.Sprintf("property_%02d", propertyIndex),
				Shape: responsepkg.ShapeSpec{
					Type: responsepkg.TypeString,
				},
			})
		}
		properties = append(properties, responsepkg.AttributeSpec{
			Name: "details",
			Shape: responsepkg.ShapeSpec{
				Type: responsepkg.TypeObject,
				Attributes: []responsepkg.AttributeSpec{
					{
						Name: "gallery",
						Shape: responsepkg.ShapeSpec{
							Type: responsepkg.TypeObject,
							Attributes: []responsepkg.AttributeSpec{
								{Name: "url", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeString}},
								{Name: "alt", Shape: responsepkg.ShapeSpec{Type: responsepkg.TypeString}},
							},
						},
					},
				},
			},
		})
		resources = append(resources, responsepkg.AttributeSpec{
			Name: fmt.Sprintf("resource_%02d", resourceIndex),
			Shape: responsepkg.ShapeSpec{
				Type:       responsepkg.TypeObject,
				Attributes: properties,
			},
		})
	}
	return responsepkg.ShapeSpec{
		Type:       responsepkg.TypeObject,
		Attributes: resources,
	}
}
