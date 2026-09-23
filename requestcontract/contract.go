// Package requestcontract couples endpoint request metadata to its live
// runtime definition and, when available, the parser that accepts the body.
package requestcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/zebodotdev/httpapi/endpoint"
	"github.com/zebodotdev/httpapi/param"
	"github.com/zebodotdev/httpapi/request"
)

// Contract is an executable endpoint request contract.
type Contract struct {
	endpoint endpoint.Endpoint
	validate func(*request.Req) *param.Error
}

// Define returns a request contract backed by parser. Define this beside the
// endpoint constructor so both use the same package-level parser.
func Define[T any](end endpoint.Endpoint, parser *param.Request[T]) Contract {
	if parser == nil {
		panic("httpapi/requestcontract: request parser is required")
	}
	validateEndpoint(end)
	return Contract{
		endpoint: end,
		validate: func(req *request.Req) *param.Error {
			_, err := parser.Parse(req)
			return err
		},
	}
}

// Declare returns a request contract backed by the endpoint's declared request
// metadata. It is appropriate for operation inventories that do not execute
// fixtures against the parser.
func Declare(end endpoint.Endpoint) Contract {
	validateEndpoint(end)
	return Contract{endpoint: end}
}

func validateEndpoint(end endpoint.Endpoint) {
	if end.Operation().ID == "" {
		panic("httpapi/requestcontract: endpoint operation ID is required")
	}
}

// Endpoint returns the live endpoint definition associated with this contract.
func (contract Contract) Endpoint() endpoint.Endpoint {
	return contract.endpoint
}

// OperationID returns the endpoint's canonical operation identifier.
func (contract Contract) OperationID() string {
	return contract.endpoint.Operation().ID
}

// Validate parses req with the endpoint's runtime request parser.
func (contract Contract) Validate(req *request.Req) *param.Error {
	if contract.validate == nil {
		panic("httpapi/requestcontract: request validator is not defined")
	}

	return contract.validate(req)
}

// Executable reports whether the contract can parse a request fixture.
func (contract Contract) Executable() bool {
	return contract.validate != nil
}

// RequestFingerprint returns a stable SHA-256 fingerprint of the endpoint's
// complete request-contract metadata, including caller availability.
func RequestFingerprint(end endpoint.Endpoint) (string, error) {
	metadata := fingerprintRequestContract(end)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal request contract metadata: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

type requestContractFingerprint struct {
	Version      int              `json:"version"`
	ContentTypes []string         `json:"content_types"`
	Required     bool             `json:"required"`
	Body         shapeFingerprint `json:"body"`
}

type shapeFingerprint struct {
	Type          param.Type                `json:"type"`
	Enum          []string                  `json:"enum,omitempty"`
	Parameters    []parameterFingerprint    `json:"parameters,omitempty"`
	Rules         []param.RuleSpec          `json:"rules,omitempty"`
	Discriminator *discriminatorFingerprint `json:"discriminator,omitempty"`
	Item          *shapeFingerprint         `json:"item,omitempty"`
}

type parameterFingerprint struct {
	Name         string           `json:"name"`
	Required     bool             `json:"required"`
	NullPolicy   param.NullPolicy `json:"null_policy"`
	Availability availability     `json:"availability"`
	Shape        shapeFingerprint `json:"shape"`
	MinSize      *int64           `json:"min_size,omitempty"`
	MaxSize      *int64           `json:"max_size,omitempty"`
	MinItems     *int64           `json:"min_items,omitempty"`
	MaxItems     *int64           `json:"max_items,omitempty"`
}

type availability struct {
	Restricted bool     `json:"restricted"`
	Callers    []string `json:"callers,omitempty"`
}

type discriminatorFingerprint struct {
	Parameter string                            `json:"parameter"`
	Variants  []discriminatorVariantFingerprint `json:"variants"`
}

type discriminatorVariantFingerprint struct {
	Value string           `json:"value"`
	Shape shapeFingerprint `json:"shape"`
}

func fingerprintRequestContract(end endpoint.Endpoint) requestContractFingerprint {
	contract := end.RequestContract()
	contentTypes := end.AcceptedContentTypes()
	accepted := make([]string, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		accepted = append(accepted, string(contentType))
	}
	return requestContractFingerprint{
		Version:      2,
		ContentTypes: accepted,
		Required:     contract.Required,
		Body:         fingerprintShape(contract.Body),
	}
}

func fingerprintShape(shape param.ShapeSpec) shapeFingerprint {
	fingerprint := shapeFingerprint{
		Type:  shape.Type,
		Enum:  shape.Enum,
		Rules: shape.Rules,
	}
	for _, parameter := range shape.Parameters {
		fingerprint.Parameters = append(
			fingerprint.Parameters,
			fingerprintParameter(parameter),
		)
	}
	if shape.Discriminator != nil {
		fingerprint.Discriminator = fingerprintDiscriminator(*shape.Discriminator)
	}
	if shape.Item != nil {
		item := fingerprintShape(*shape.Item)
		fingerprint.Item = &item
	}
	return fingerprint
}

func fingerprintParameter(parameter param.ParameterSpec) parameterFingerprint {
	callers := parameter.Availability.Callers()
	callerNames := make([]string, 0, len(callers))
	for _, requestCaller := range callers {
		callerNames = append(callerNames, requestCaller.Name())
	}
	return parameterFingerprint{
		Name:       parameter.Name,
		Required:   parameter.Required,
		NullPolicy: parameter.NullPolicy,
		Availability: availability{
			Restricted: parameter.Availability.Restricted(),
			Callers:    callerNames,
		},
		Shape:    fingerprintShape(parameter.Shape),
		MinSize:  parameter.MinSize,
		MaxSize:  parameter.MaxSize,
		MinItems: parameter.MinItems,
		MaxItems: parameter.MaxItems,
	}
}

func fingerprintDiscriminator(
	discriminator param.DiscriminatorSpec,
) *discriminatorFingerprint {
	fingerprint := &discriminatorFingerprint{Parameter: discriminator.Parameter}
	for _, variant := range discriminator.Variants {
		fingerprint.Variants = append(
			fingerprint.Variants,
			discriminatorVariantFingerprint{
				Value: variant.Value,
				Shape: fingerprintShape(variant.Shape),
			},
		)
	}
	return fingerprint
}
