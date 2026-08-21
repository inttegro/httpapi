package schema

import (
	"slices"

	"github.com/zebodotdev/httpapi/openapi/spec"
	parampkg "github.com/zebodotdev/httpapi/param"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

// FromParamShape translates a provider-neutral param shape into an OpenAPI
// schema.
func FromParamShape(shape parampkg.ShapeSpec) spec.Schema {
	schema := paramTypeSchema(shape.Type)
	if len(shape.Enum) > 0 {
		schema.Enum = cloneStringSlice(shape.Enum)
	}
	switch shape.Type {
	case parampkg.TypeObject:
		if shape.Discriminator != nil {
			schema.OneOf = discriminatorOneOf(shape.Discriminator, FromParamShape)
			schema.Discriminator = &spec.Discriminator{
				PropertyName: shape.Discriminator.Parameter,
			}
		} else {
			schema.Properties = paramProperties(shape.Parameters)
			schema.Required = paramRequired(shape.Parameters)
			schema.AllOf = paramRules(shape.Rules)
		}
	case parampkg.TypeArray:
		schema.Items = paramItemSchema(shape.Item)
	}
	return schema
}

// FromParamShapeSwagger2 translates a param shape into the Swagger 2.0 subset
// used by gateway documents.
//
// Swagger 2.0 cannot express oneOf. Discriminated objects are therefore
// downgraded into a legal object schema containing the discriminator enum plus
// the union of variant properties. Runtime parsing still enforces the exact
// branch contract.
func FromParamShapeSwagger2(shape parampkg.ShapeSpec) spec.Schema {
	schema := paramTypeSchema(shape.Type)
	if len(shape.Enum) > 0 {
		schema.Enum = cloneStringSlice(shape.Enum)
	}
	switch shape.Type {
	case parampkg.TypeObject:
		if shape.Discriminator != nil {
			schema.Properties = discriminatorSwaggerProperties(shape.Discriminator)
			schema.Required = []string{shape.Discriminator.Parameter}
		} else {
			schema.Properties = paramPropertiesWith(shape.Parameters, FromParamShapeSwagger2)
			schema.Required = paramRequired(shape.Parameters)
		}
	case parampkg.TypeArray:
		schema.Items = paramItemSchemaWith(shape.Item, FromParamShapeSwagger2)
	}
	return schema
}

// FromResponseShape translates a provider-neutral response shape into an
// OpenAPI schema.
func FromResponseShape(shape responsepkg.ShapeSpec) spec.Schema {
	schema := responseTypeSchema(shape.Type)
	if shape.Format != "" {
		schema.Format = shape.Format
	}
	switch shape.Type {
	case responsepkg.TypeObject:
		if shape.MapValue != nil {
			schema.AdditionalProperties = responseMapValueSchema(shape.MapValue)
		} else {
			schema.Properties = responseProperties(shape.Attributes)
			schema.Required = responseRequired(shape.Attributes)
		}
	case responsepkg.TypeArray:
		schema.Items = responseItemSchema(shape.Item)
	}
	return schema
}

func paramTypeSchema(typ parampkg.Type) spec.Schema {
	switch typ {
	case parampkg.TypeString:
		return spec.Schema{Type: "string"}
	case parampkg.TypeInt:
		return spec.Schema{Type: "integer", Format: "int32"}
	case parampkg.TypeInt64:
		return spec.Schema{Type: "integer", Format: "int64"}
	case parampkg.TypeFloat64:
		return spec.Schema{Type: "number", Format: "double"}
	case parampkg.TypeBool:
		return spec.Schema{Type: "boolean"}
	case parampkg.TypeObject:
		return spec.Schema{Type: "object"}
	case parampkg.TypeArray:
		return spec.Schema{Type: "array"}
	default:
		return spec.Schema{}
	}
}

func responseTypeSchema(typ responsepkg.Type) spec.Schema {
	switch typ {
	case responsepkg.TypeString:
		return spec.Schema{Type: "string"}
	case responsepkg.TypeInt:
		return spec.Schema{Type: "integer", Format: "int32"}
	case responsepkg.TypeFloat64:
		return spec.Schema{Type: "number", Format: "double"}
	case responsepkg.TypeBool:
		return spec.Schema{Type: "boolean"}
	case responsepkg.TypeObject:
		return spec.Schema{Type: "object"}
	case responsepkg.TypeArray:
		return spec.Schema{Type: "array"}
	default:
		return spec.Schema{}
	}
}

func paramProperties(parameters []parampkg.ParameterSpec) map[string]spec.Schema {
	return paramPropertiesWith(parameters, FromParamShape)
}

func paramPropertiesWith(
	parameters []parampkg.ParameterSpec,
	convert func(parampkg.ShapeSpec) spec.Schema,
) map[string]spec.Schema {
	if len(parameters) == 0 {
		return nil
	}

	properties := make(map[string]spec.Schema, len(parameters))
	for _, parameter := range parameters {
		properties[parameter.Name] = paramPropertySchema(parameter, convert)
	}
	return properties
}

func paramPropertySchema(
	parameter parampkg.ParameterSpec,
	convert func(parampkg.ShapeSpec) spec.Schema,
) spec.Schema {
	schema := convert(parameter.Shape)
	return applyParamBounds(schema, parameter)
}

func applyParamBounds(schema spec.Schema, parameter parampkg.ParameterSpec) spec.Schema {
	switch parameter.Shape.Type {
	case parampkg.TypeString:
		schema.MinLength = cloneInt64Pointer(parameter.MinSize)
		schema.MaxLength = cloneInt64Pointer(parameter.MaxSize)
	case parampkg.TypeInt, parampkg.TypeInt64, parampkg.TypeFloat64:
		schema.Minimum = cloneInt64Pointer(parameter.MinSize)
		schema.Maximum = cloneInt64Pointer(parameter.MaxSize)
	case parampkg.TypeObject:
		schema.MinProperties = cloneInt64Pointer(parameter.MinSize)
		schema.MaxProperties = cloneInt64Pointer(parameter.MaxSize)
	case parampkg.TypeArray:
		schema.MinItems = cloneFirstInt64Pointer(parameter.MinItems, parameter.MinSize)
		schema.MaxItems = cloneFirstInt64Pointer(parameter.MaxItems, parameter.MaxSize)
	}
	return schema
}

func paramRequired(parameters []parampkg.ParameterSpec) []string {
	required := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.Required {
			required = append(required, parameter.Name)
		}
	}
	if len(required) == 0 {
		return nil
	}
	return required
}

func cloneFirstInt64Pointer(values ...*int64) *int64 {
	for _, value := range values {
		if value != nil {
			return cloneInt64Pointer(value)
		}
	}
	return nil
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func paramRules(rules []parampkg.RuleSpec) []spec.Schema {
	if len(rules) == 0 {
		return nil
	}

	allOf := make([]spec.Schema, 0, len(rules))
	for _, rule := range rules {
		allOf = append(allOf, paramRuleSchemas(rule)...)
	}
	if len(allOf) == 0 {
		return nil
	}
	return allOf
}

func paramRuleSchemas(rule parampkg.RuleSpec) []spec.Schema {
	if len(rule.Names) == 0 {
		return nil
	}

	if rule.MinPresent == 1 && rule.MaxPresent == 1 {
		return []spec.Schema{{OneOf: requiredNameSchemas(rule.Names, 1)}}
	}

	schemas := []spec.Schema{}
	if rule.MinPresent > 0 {
		schemas = append(schemas, spec.Schema{
			AnyOf: requiredNameSchemas(rule.Names, rule.MinPresent),
		})
	}
	if rule.MaxPresent > 0 && rule.MaxPresent < len(rule.Names) {
		for _, names := range nameCombinations(rule.Names, rule.MaxPresent+1) {
			schemas = append(schemas, spec.Schema{
				Not: &spec.Schema{Required: names},
			})
		}
	}

	return schemas
}

func requiredNameSchemas(names []string, count int) []spec.Schema {
	combinations := nameCombinations(names, count)
	schemas := make([]spec.Schema, 0, len(combinations))
	for _, combination := range combinations {
		schemas = append(schemas, spec.Schema{Required: combination})
	}
	return schemas
}

func nameCombinations(names []string, count int) [][]string {
	if count <= 0 || count > len(names) {
		return nil
	}

	combinations := [][]string{}
	var walk func(start int, selected []string)
	walk = func(start int, selected []string) {
		if len(selected) == count {
			combination := make([]string, count)
			copy(combination, selected)
			combinations = append(combinations, combination)
			return
		}
		remaining := count - len(selected)
		for i := start; i <= len(names)-remaining; i++ {
			walk(i+1, append(selected, names[i]))
		}
	}
	walk(0, nil)

	return combinations
}

func paramItemSchema(shape *parampkg.ShapeSpec) *spec.Schema {
	return paramItemSchemaWith(shape, FromParamShape)
}

func paramItemSchemaWith(
	shape *parampkg.ShapeSpec,
	convert func(parampkg.ShapeSpec) spec.Schema,
) *spec.Schema {
	if shape == nil {
		schema := spec.Schema{}
		return &schema
	}
	schema := convert(*shape)
	return &schema
}

func discriminatorOneOf(
	discriminator *parampkg.DiscriminatorSpec,
	convert func(parampkg.ShapeSpec) spec.Schema,
) []spec.Schema {
	if discriminator == nil || discriminator.Parameter == "" || len(discriminator.Variants) == 0 {
		return nil
	}

	branches := make([]spec.Schema, 0, len(discriminator.Variants))
	for _, variant := range discriminator.Variants {
		branch := convert(variant.Shape)
		if branch.Type == "" {
			branch.Type = "object"
		}
		if branch.Properties == nil {
			branch.Properties = map[string]spec.Schema{}
		}
		branch.Properties[discriminator.Parameter] = spec.Schema{
			Type: "string",
			Enum: []string{variant.Value},
		}
		branch.Required = appendUniqueString(branch.Required, discriminator.Parameter)
		branches = append(branches, branch)
	}
	return branches
}

func discriminatorSwaggerProperties(discriminator *parampkg.DiscriminatorSpec) map[string]spec.Schema {
	if discriminator == nil || discriminator.Parameter == "" {
		return nil
	}

	properties := map[string]spec.Schema{
		discriminator.Parameter: {
			Type: "string",
			Enum: discriminatorValues(discriminator),
		},
	}
	for _, variant := range discriminator.Variants {
		shape := FromParamShapeSwagger2(variant.Shape)
		for name, property := range shape.Properties {
			if name == discriminator.Parameter {
				continue
			}
			if _, ok := properties[name]; ok {
				continue
			}
			properties[name] = property
		}
	}
	return properties
}

func discriminatorValues(discriminator *parampkg.DiscriminatorSpec) []string {
	values := make([]string, 0, len(discriminator.Variants))
	for _, variant := range discriminator.Variants {
		values = append(values, variant.Value)
	}
	return values
}

func responseProperties(attributes []responsepkg.AttributeSpec) map[string]spec.Schema {
	if len(attributes) == 0 {
		return nil
	}

	properties := make(map[string]spec.Schema, len(attributes))
	for _, attribute := range attributes {
		properties[attribute.Name] = FromResponseShape(attribute.Shape)
	}
	return properties
}

func responseRequired(attributes []responsepkg.AttributeSpec) []string {
	required := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		if attribute.Required {
			required = append(required, attribute.Name)
		}
	}
	if len(required) == 0 {
		return nil
	}
	return required
}

func responseItemSchema(shape *responsepkg.ShapeSpec) *spec.Schema {
	if shape == nil {
		schema := spec.Schema{}
		return &schema
	}
	schema := FromResponseShape(*shape)
	return &schema
}

func responseMapValueSchema(shape *responsepkg.ShapeSpec) *spec.Schema {
	if shape == nil {
		schema := spec.Schema{}
		return &schema
	}
	schema := FromResponseShape(*shape)
	return &schema
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func appendUniqueString(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}
