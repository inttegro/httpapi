package endpoint

import (
	"fmt"
	"net/http"
	"strings"

	callerpkg "github.com/zebodotdev/httpapi/caller"
	parampkg "github.com/zebodotdev/httpapi/param"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

// RequestContract describes the request payload accepted by an endpoint.
type RequestContract struct {
	// Body is the provider-neutral request payload description. JSON endpoints
	// derive it from a param request parser; other content types may declare the
	// accepted fields directly.
	Body parampkg.ShapeSpec

	// Required reports whether the endpoint requires a request body.
	Required bool
}

// ResponseContract describes one response payload emitted by an endpoint.
type ResponseContract struct {
	// Status is the HTTP response status code. Leave zero for a default response.
	Status int

	// Description is the human-readable response description used by generated
	// API documents.
	Description string

	// ContentType is the response body media type. It defaults to
	// application/json when Body is set.
	ContentType ContentType

	// Body is the provider-neutral JSON response body description derived from a
	// response shape.
	Body responsepkg.ShapeSpec
}

// RequestBody returns a request contract from a param request parser.
func RequestBody[T any](request *parampkg.Request[T]) RequestContract {
	return RequestContract{
		Body:     parampkg.Describe(request).Body,
		Required: true,
	}
}

// ResponseBody returns a JSON response contract from a response shape.
func ResponseBody[T any](
	status int,
	description string,
	shape responsepkg.Shape[T],
) ResponseContract {
	return ResponseContract{
		Status:      status,
		Description: description,
		ContentType: ApplicationJson,
		Body:        responsepkg.Describe(shape),
	}
}

// NoContentResponse returns a response contract without a response body.
func NoContentResponse(status int, description string) ResponseContract {
	return ResponseContract{
		Status:      status,
		Description: description,
	}
}

// RequestContract returns the endpoint's request payload contract.
func (e Endpoint) RequestContract() RequestContract {
	return cloneRequestContract(e.requestContract)
}

// ResponseContracts returns the endpoint's response payload contracts.
func (e Endpoint) ResponseContracts() []ResponseContract {
	return cloneResponseContracts(e.responseContracts)
}

func normalizeRequestContract(contract RequestContract) RequestContract {
	contract.Body = cloneParamShapeSpec(contract.Body)
	return contract
}

func normalizeResponseContracts(contracts []ResponseContract) []ResponseContract {
	if len(contracts) == 0 {
		return nil
	}

	out := make([]ResponseContract, 0, len(contracts))
	for _, contract := range contracts {
		out = append(out, normalizeResponseContract(contract))
	}
	return out
}

func normalizeResponseContract(contract ResponseContract) ResponseContract {
	if contract.Status != 0 &&
		(contract.Status < http.StatusContinue || contract.Status > 599) {
		panic(fmt.Sprintf("httpapi: response status is invalid: %d", contract.Status))
	}

	contract.Description = strings.TrimSpace(contract.Description)
	if contract.Body.Type != "" && contract.ContentType == "" {
		contract.ContentType = ApplicationJson
	}
	if contract.ContentType != "" {
		contract.ContentType = normalizeEndpointContentType(contract.ContentType)
	}
	contract.Body = cloneResponseShapeSpec(contract.Body)

	return contract
}

func cloneRequestContract(contract RequestContract) RequestContract {
	return normalizeRequestContract(contract)
}

func cloneResponseContracts(contracts []ResponseContract) []ResponseContract {
	return normalizeResponseContracts(contracts)
}

func cloneParamShapeSpec(spec parampkg.ShapeSpec) parampkg.ShapeSpec {
	cloned := parampkg.ShapeSpec{
		Type:          spec.Type,
		Enum:          cloneStringSlice(spec.Enum),
		Discriminator: cloneParamDiscriminatorSpec(spec.Discriminator),
		Item:          cloneParamShapeSpecPointer(spec.Item),
	}
	if len(spec.Parameters) > 0 {
		cloned.Parameters = make([]parampkg.ParameterSpec, 0, len(spec.Parameters))
		for _, parameter := range spec.Parameters {
			cloned.Parameters = append(cloned.Parameters, cloneParamParameterSpec(parameter))
		}
	}
	if len(spec.Rules) > 0 {
		cloned.Rules = make([]parampkg.RuleSpec, 0, len(spec.Rules))
		for _, rule := range spec.Rules {
			cloned.Rules = append(cloned.Rules, cloneParamRuleSpec(rule))
		}
	}
	return cloned
}

func cloneParamDiscriminatorSpec(
	spec *parampkg.DiscriminatorSpec,
) *parampkg.DiscriminatorSpec {
	if spec == nil {
		return nil
	}
	cloned := &parampkg.DiscriminatorSpec{
		Parameter: spec.Parameter,
	}
	if len(spec.Variants) > 0 {
		cloned.Variants = make([]parampkg.DiscriminatorVariantSpec, 0, len(spec.Variants))
		for _, variant := range spec.Variants {
			cloned.Variants = append(cloned.Variants, parampkg.DiscriminatorVariantSpec{
				Value: variant.Value,
				Shape: cloneParamShapeSpec(variant.Shape),
			})
		}
	}
	return cloned
}

func cloneParamShapeSpecPointer(spec *parampkg.ShapeSpec) *parampkg.ShapeSpec {
	if spec == nil {
		return nil
	}
	cloned := cloneParamShapeSpec(*spec)
	return &cloned
}

func cloneParamParameterSpec(spec parampkg.ParameterSpec) parampkg.ParameterSpec {
	spec.Availability = cloneCallerSet(spec.Availability)
	spec.Shape = cloneParamShapeSpec(spec.Shape)
	spec.MinSize = cloneInt64Pointer(spec.MinSize)
	spec.MaxSize = cloneInt64Pointer(spec.MaxSize)
	spec.MinItems = cloneInt64Pointer(spec.MinItems)
	spec.MaxItems = cloneInt64Pointer(spec.MaxItems)
	return spec
}

func cloneParamRuleSpec(spec parampkg.RuleSpec) parampkg.RuleSpec {
	if len(spec.Names) > 0 {
		names := make([]string, len(spec.Names))
		copy(names, spec.Names)
		spec.Names = names
	}
	return spec
}

func cloneResponseShapeSpec(spec responsepkg.ShapeSpec) responsepkg.ShapeSpec {
	cloned := responsepkg.ShapeSpec{
		Type:     spec.Type,
		Format:   spec.Format,
		Enum:     cloneStringSlice(spec.Enum),
		Item:     cloneResponseShapeSpecPointer(spec.Item),
		MapValue: cloneResponseShapeSpecPointer(spec.MapValue),
	}
	if len(spec.Attributes) > 0 {
		cloned.Attributes = make([]responsepkg.AttributeSpec, 0, len(spec.Attributes))
		for _, attribute := range spec.Attributes {
			cloned.Attributes = append(cloned.Attributes, cloneResponseAttributeSpec(attribute))
		}
	}
	return cloned
}

func cloneResponseShapeSpecPointer(spec *responsepkg.ShapeSpec) *responsepkg.ShapeSpec {
	if spec == nil {
		return nil
	}
	cloned := cloneResponseShapeSpec(*spec)
	return &cloned
}

func cloneResponseAttributeSpec(spec responsepkg.AttributeSpec) responsepkg.AttributeSpec {
	spec.Availability = cloneCallerSet(spec.Availability)
	spec.Shape = cloneResponseShapeSpec(spec.Shape)
	return spec
}

func cloneCallerSet(set callerpkg.Set) callerpkg.Set {
	if !set.Restricted() {
		return callerpkg.Set{}
	}
	return callerpkg.SetOf(set.Callers()...)
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
