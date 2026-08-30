package schema

import (
	"reflect"
	"testing"

	"github.com/zebodotdev/httpapi/param"
	"github.com/zebodotdev/httpapi/response"
)

func TestFromParamShapeIncludesStringEnum(t *testing.T) {
	shape := param.ShapeSpec{
		Type: param.TypeString,
		Enum: []string{"product", "fee"},
	}

	got := FromParamShape(shape)
	if got.Type != "string" {
		t.Fatalf("type = %q, want string", got.Type)
	}
	if !reflect.DeepEqual(got.Enum, []string{"product", "fee"}) {
		t.Fatalf("enum = %#v", got.Enum)
	}

	shape.Enum[0] = "mutated"
	if got.Enum[0] != "product" {
		t.Fatalf("enum leaked input mutation: %#v", got.Enum)
	}
}

func TestFromParamShapeIncludesParameterBounds(t *testing.T) {
	min := int64(1)
	max := int64(9)
	minItems := int64(2)
	got := FromParamShape(param.ShapeSpec{
		Type: param.TypeObject,
		Parameters: []param.ParameterSpec{
			{
				Name:    "name",
				Shape:   param.ShapeSpec{Type: param.TypeString},
				MinSize: &min,
				MaxSize: &max,
			},
			{
				Name:    "count",
				Shape:   param.ShapeSpec{Type: param.TypeInt},
				MinSize: &min,
			},
			{
				Name: "tags",
				Shape: param.ShapeSpec{
					Type: param.TypeArray,
					Item: &param.ShapeSpec{Type: param.TypeString},
				},
				MinItems: &minItems,
				MaxSize:  &max,
			},
			{
				Name:    "metadata",
				Shape:   param.ShapeSpec{Type: param.TypeObject},
				MaxSize: &max,
			},
		},
	})

	name := got.Properties["name"]
	if name.MinLength == nil || *name.MinLength != 1 ||
		name.MaxLength == nil || *name.MaxLength != 9 {
		t.Fatalf("name bounds = %#v", name)
	}
	count := got.Properties["count"]
	if count.Minimum == nil || *count.Minimum != 1 {
		t.Fatalf("count bounds = %#v", count)
	}
	tags := got.Properties["tags"]
	if tags.MinItems == nil || *tags.MinItems != 2 ||
		tags.MaxItems == nil || *tags.MaxItems != 9 {
		t.Fatalf("tags bounds = %#v", tags)
	}
	metadata := got.Properties["metadata"]
	if metadata.MaxProperties == nil || *metadata.MaxProperties != 9 {
		t.Fatalf("metadata bounds = %#v", metadata)
	}

	min = 100
	if *got.Properties["name"].MinLength != 1 ||
		*got.Properties["count"].Minimum != 1 {
		t.Fatalf("schema leaked bound pointer mutation: %#v", got.Properties)
	}
}

func TestFromParamShapeIncludesDiscriminatorBranches(t *testing.T) {
	got := FromParamShape(param.ShapeSpec{
		Type: param.TypeObject,
		Discriminator: &param.DiscriminatorSpec{
			Parameter: "type",
			Variants: []param.DiscriminatorVariantSpec{
				{
					Value: "product",
					Shape: param.ShapeSpec{
						Type: param.TypeObject,
						Parameters: []param.ParameterSpec{{
							Name:     "name",
							Required: true,
							Shape:    param.ShapeSpec{Type: param.TypeString},
						}},
					},
				},
				{
					Value: "fee",
					Shape: param.ShapeSpec{
						Type: param.TypeObject,
						Parameters: []param.ParameterSpec{{
							Name:     "amount",
							Required: true,
							Shape:    param.ShapeSpec{Type: param.TypeInt},
						}},
					},
				},
			},
		},
	})

	if got.Discriminator == nil || got.Discriminator.PropertyName != "type" {
		t.Fatalf("discriminator = %#v", got.Discriminator)
	}
	if len(got.OneOf) != 2 {
		t.Fatalf("oneOf = %#v", got.OneOf)
	}
	product := got.OneOf[0]
	if product.Properties["type"].Type != "string" ||
		!reflect.DeepEqual(product.Properties["type"].Enum, []string{"product"}) {
		t.Fatalf("product discriminator property = %#v", product.Properties["type"])
	}
	if product.Properties["name"].Type != "string" {
		t.Fatalf("product name property = %#v", product.Properties["name"])
	}
	if !reflect.DeepEqual(product.Required, []string{"name", "type"}) {
		t.Fatalf("product required = %#v", product.Required)
	}
	fee := got.OneOf[1]
	if fee.Properties["amount"].Type != "integer" {
		t.Fatalf("fee amount property = %#v", fee.Properties["amount"])
	}
}

func TestFromParamShapeIncludesPresenceRuleComposition(t *testing.T) {
	got := FromParamShape(param.ShapeSpec{
		Type: param.TypeObject,
		Parameters: []param.ParameterSpec{
			{Name: "product", Shape: param.ShapeSpec{Type: param.TypeObject}},
			{Name: "product_id", Shape: param.ShapeSpec{Type: param.TypeString}},
			{Name: "quantity", Required: true, Shape: param.ShapeSpec{Type: param.TypeObject}},
		},
		Rules: []param.RuleSpec{{
			Names:      []string{"product", "product_id"},
			MinPresent: 1,
			MaxPresent: 1,
		}},
	})

	if !reflect.DeepEqual(got.Required, []string{"quantity"}) {
		t.Fatalf("required = %#v, want quantity", got.Required)
	}
	if len(got.AllOf) != 1 {
		t.Fatalf("allOf = %#v, want one rule schema", got.AllOf)
	}
	oneOf := got.AllOf[0].OneOf
	if len(oneOf) != 2 {
		t.Fatalf("oneOf = %#v, want product selector branches", oneOf)
	}
	if !reflect.DeepEqual(oneOf[0].Required, []string{"product"}) ||
		!reflect.DeepEqual(oneOf[1].Required, []string{"product_id"}) {
		t.Fatalf("oneOf required branches = %#v", oneOf)
	}
}

func TestFromParamShapeIncludesAtLeastOneRuleComposition(t *testing.T) {
	got := FromParamShape(param.ShapeSpec{
		Type: param.TypeObject,
		Parameters: []param.ParameterSpec{
			{Name: "email", Shape: param.ShapeSpec{Type: param.TypeString}},
			{Name: "phone", Shape: param.ShapeSpec{Type: param.TypeString}},
		},
		Rules: []param.RuleSpec{{
			Names:      []string{"email", "phone"},
			MinPresent: 1,
		}},
	})

	if len(got.AllOf) != 1 {
		t.Fatalf("allOf = %#v, want one rule schema", got.AllOf)
	}
	anyOf := got.AllOf[0].AnyOf
	if len(anyOf) != 2 {
		t.Fatalf("anyOf = %#v, want contact selector branches", anyOf)
	}
	if !reflect.DeepEqual(anyOf[0].Required, []string{"email"}) ||
		!reflect.DeepEqual(anyOf[1].Required, []string{"phone"}) {
		t.Fatalf("anyOf required branches = %#v", anyOf)
	}
}

func TestFromParamShapeIncludesAtMostOneRuleComposition(t *testing.T) {
	got := FromParamShape(param.ShapeSpec{
		Type: param.TypeObject,
		Parameters: []param.ParameterSpec{
			{Name: "single_use", Shape: param.ShapeSpec{Type: param.TypeBool}},
			{Name: "multi_use", Shape: param.ShapeSpec{Type: param.TypeBool}},
		},
		Rules: []param.RuleSpec{{
			Names:      []string{"single_use", "multi_use"},
			MaxPresent: 1,
		}},
	})

	if len(got.AllOf) != 1 || got.AllOf[0].Not == nil {
		t.Fatalf("allOf = %#v, want pairwise not rule", got.AllOf)
	}
	if !reflect.DeepEqual(got.AllOf[0].Not.Required, []string{"single_use", "multi_use"}) {
		t.Fatalf("not required = %#v", got.AllOf[0].Not.Required)
	}
}

func TestFromParamShapeSwagger2DowngradesDiscriminator(t *testing.T) {
	got := FromParamShapeSwagger2(param.ShapeSpec{
		Type: param.TypeObject,
		Discriminator: &param.DiscriminatorSpec{
			Parameter: "type",
			Variants: []param.DiscriminatorVariantSpec{
				{
					Value: "product",
					Shape: param.ShapeSpec{
						Type: param.TypeObject,
						Parameters: []param.ParameterSpec{{
							Name:     "name",
							Required: true,
							Shape:    param.ShapeSpec{Type: param.TypeString},
						}},
					},
				},
				{
					Value: "fee",
					Shape: param.ShapeSpec{
						Type: param.TypeObject,
						Parameters: []param.ParameterSpec{{
							Name:     "amount",
							Required: true,
							Shape:    param.ShapeSpec{Type: param.TypeInt},
						}},
					},
				},
			},
		},
	})

	if got.Discriminator != nil || len(got.OneOf) != 0 {
		t.Fatalf("swagger schema contains OpenAPI 3 discriminator data: %#v", got)
	}
	if got.Properties["type"].Type != "string" ||
		!reflect.DeepEqual(got.Properties["type"].Enum, []string{"product", "fee"}) {
		t.Fatalf("type property = %#v", got.Properties["type"])
	}
	if got.Properties["name"].Type != "string" {
		t.Fatalf("name property = %#v", got.Properties["name"])
	}
	if got.Properties["amount"].Type != "integer" {
		t.Fatalf("amount property = %#v", got.Properties["amount"])
	}
	if !reflect.DeepEqual(got.Required, []string{"type"}) {
		t.Fatalf("required = %#v", got.Required)
	}
}

func TestFromResponseShapeIncludesMapAdditionalProperties(t *testing.T) {
	got := FromResponseShape(response.ShapeSpec{
		Type: response.TypeObject,
		MapValue: &response.ShapeSpec{
			Type: response.TypeObject,
			Attributes: []response.AttributeSpec{
				{
					Name:     "status",
					Required: true,
					Shape:    response.ShapeSpec{Type: response.TypeString},
				},
				{
					Name:  "count",
					Shape: response.ShapeSpec{Type: response.TypeInt},
				},
			},
		},
	})

	if got.Type != "object" {
		t.Fatalf("type = %q, want object", got.Type)
	}
	if got.AdditionalProperties == nil {
		t.Fatal("additionalProperties is nil")
	}
	if got.Properties != nil || got.Required != nil {
		t.Fatalf("map schema has fixed object properties: %#v", got)
	}

	value := got.AdditionalProperties
	if value.Type != "object" {
		t.Fatalf("additionalProperties type = %q, want object", value.Type)
	}
	if value.Properties["status"].Type != "string" {
		t.Fatalf("status schema = %#v", value.Properties["status"])
	}
	if value.Properties["count"].Type != "integer" {
		t.Fatalf("count schema = %#v", value.Properties["count"])
	}
	if !reflect.DeepEqual(value.Required, []string{"status"}) {
		t.Fatalf("required = %#v, want status", value.Required)
	}
}

func TestFromResponseShapeIncludesStringEnum(t *testing.T) {
	shape := response.ShapeSpec{
		Type: response.TypeString,
		Enum: []string{"pending", "succeeded"},
	}
	got := FromResponseShape(shape)
	if !reflect.DeepEqual(got.Enum, shape.Enum) {
		t.Fatalf("enum = %#v, want %#v", got.Enum, shape.Enum)
	}

	shape.Enum[0] = "mutated"
	if got.Enum[0] != "pending" {
		t.Fatalf("response enum leaked input mutation: %#v", got.Enum)
	}
}
