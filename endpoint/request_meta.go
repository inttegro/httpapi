package endpoint

import parampkg "github.com/inttegro/httpapi/param"

const standardRequestMetaParameter = "request_meta"

func withStandardRequestMeta(
	body parampkg.ShapeSpec,
	idempotent bool,
) parampkg.ShapeSpec {
	if body.Type != parampkg.TypeObject || body.MapValue != nil || body.Discriminator != nil {
		return body
	}

	standard := standardRequestMetaSpec(idempotent)
	for idx := range body.Parameters {
		if body.Parameters[idx].Name != standardRequestMetaParameter {
			continue
		}
		body.Parameters[idx].Shape = mergeObjectShape(
			body.Parameters[idx].Shape,
			standard.Shape,
		)
		return body
	}

	body.Parameters = append(body.Parameters, standard)
	return body
}

func standardRequestMetaSpec(idempotent bool) parampkg.ParameterSpec {
	parameters := []parampkg.ParameterSpec{}
	if idempotent {
		parameters = append(parameters, parampkg.ParameterSpec{
			Name:  "idempotency_key",
			Shape: parampkg.ShapeSpec{Type: parampkg.TypeString},
		})
	}
	parameters = append(parameters, parampkg.ParameterSpec{
		Name: "response",
		Shape: parampkg.ShapeSpec{
			Type: parampkg.TypeObject,
			Parameters: []parampkg.ParameterSpec{{
				Name:  "projection",
				Shape: responseProjectionShapeSpec(),
			}},
		},
	})

	return parampkg.ParameterSpec{
		Name: standardRequestMetaParameter,
		Shape: parampkg.ShapeSpec{
			Type:       parampkg.TypeObject,
			Parameters: parameters,
		},
	}
}

func responseProjectionShapeSpec() parampkg.ShapeSpec {
	minimumItems := int64(1)
	propertyPaths := func(name string) parampkg.ParameterSpec {
		return parampkg.ParameterSpec{
			Name: name,
			Shape: parampkg.ShapeSpec{
				Type: parampkg.TypeArray,
				Item: &parampkg.ShapeSpec{Type: parampkg.TypeString},
			},
			MinItems: &minimumItems,
		}
	}
	resourceProjection := parampkg.ShapeSpec{
		Type: parampkg.TypeObject,
		Parameters: []parampkg.ParameterSpec{
			propertyPaths("pick"),
			propertyPaths("omit"),
		},
		Rules: []parampkg.RuleSpec{{
			Names:      []string{"pick", "omit"},
			MaxPresent: 1,
		}},
	}
	return parampkg.ShapeSpec{
		Type:     parampkg.TypeObject,
		MapValue: &resourceProjection,
	}
}

func mergeObjectShape(
	existing parampkg.ShapeSpec,
	standard parampkg.ShapeSpec,
) parampkg.ShapeSpec {
	if existing.Type != parampkg.TypeObject ||
		existing.MapValue != nil ||
		existing.Discriminator != nil {
		return existing
	}

	for _, parameter := range standard.Parameters {
		matched := false
		for idx := range existing.Parameters {
			if existing.Parameters[idx].Name != parameter.Name {
				continue
			}
			existing.Parameters[idx].Shape = mergeObjectShape(
				existing.Parameters[idx].Shape,
				parameter.Shape,
			)
			matched = true
			break
		}
		if !matched {
			existing.Parameters = append(existing.Parameters, parameter)
		}
	}
	return existing
}
