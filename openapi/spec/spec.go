package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
)

// ErrInvalidExtensionName reports an OpenAPI extension name that does not begin
// with x-.
var ErrInvalidExtensionName = errors.New(
	"openapi/spec: extension name must begin with x-",
)

// ErrDuplicateExtensionName reports two extensions that normalize to the same
// OpenAPI extension name.
var ErrDuplicateExtensionName = errors.New(
	"openapi/spec: duplicate extension name",
)

// Document is the document-level schema produced from endpoints.
type Document struct {
	// OpenAPI is the OpenAPI 3.x version string.
	OpenAPI string `json:"openapi,omitempty" yaml:"openapi,omitempty"`

	// Swagger is the Swagger 2.0 version string.
	Swagger string `json:"swagger,omitempty" yaml:"swagger,omitempty"`

	// Info is the document metadata block.
	Info Info `json:"info" yaml:"info"`

	// Servers contains OpenAPI 3 server entries.
	Servers []Server `json:"servers,omitempty" yaml:"servers,omitempty"`

	// Host is the Swagger 2.0 host value.
	Host string `json:"host,omitempty" yaml:"host,omitempty"`

	// Schemes is the Swagger 2.0 list of accepted URL schemes.
	Schemes []string `json:"schemes,omitempty" yaml:"schemes,omitempty"`

	// Produces is the Swagger 2.0 document-level list of response media types.
	Produces []string `json:"produces,omitempty" yaml:"produces,omitempty"`

	// Paths maps URL paths to operations.
	Paths Paths `json:"paths" yaml:"paths"`
}

// Info is the generated document info block.
type Info struct {
	// Title is the human-readable API document title.
	Title string `json:"title" yaml:"title"`

	// Description is optional document-level API context.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// TermsOfService is an optional URL for the API terms of service.
	TermsOfService string `json:"termsOfService,omitempty" yaml:"termsOfService,omitempty"`

	// Contact identifies the people or organization responsible for the API.
	Contact *Contact `json:"contact,omitempty" yaml:"contact,omitempty"`

	// License describes the API document license.
	License *License `json:"license,omitempty" yaml:"license,omitempty"`

	// Version is the service or API version represented by the document.
	Version string `json:"version" yaml:"version"`
}

// Contact is the OpenAPI contact object.
type Contact struct {
	// Name is the contact display name.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// URL is the contact information URL.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`

	// Email is the contact email address.
	Email string `json:"email,omitempty" yaml:"email,omitempty"`
}

// License is the OpenAPI license object.
type License struct {
	// Name is the license name.
	Name string `json:"name" yaml:"name"`

	// URL is the optional URL for the license text.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
}

// Server is an OpenAPI 3 server entry.
type Server struct {
	// URL is the base URL for the API server.
	URL string `json:"url" yaml:"url"`

	// Description is optional human-readable context for this server URL.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// NormalizeInfo trims optional string fields and drops empty nested objects.
func NormalizeInfo(info Info) Info {
	info.Title = strings.TrimSpace(info.Title)
	info.Description = strings.TrimSpace(info.Description)
	info.TermsOfService = strings.TrimSpace(info.TermsOfService)
	info.Version = strings.TrimSpace(info.Version)
	if info.Contact != nil {
		contact := *info.Contact
		contact.Name = strings.TrimSpace(contact.Name)
		contact.URL = strings.TrimSpace(contact.URL)
		contact.Email = strings.TrimSpace(contact.Email)
		if contact.Name == "" && contact.URL == "" && contact.Email == "" {
			info.Contact = nil
		} else {
			info.Contact = &contact
		}
	}
	if info.License != nil {
		license := *info.License
		license.Name = strings.TrimSpace(license.Name)
		license.URL = strings.TrimSpace(license.URL)
		if license.Name == "" && license.URL == "" {
			info.License = nil
		} else {
			info.License = &license
		}
	}

	return info
}

// NormalizeServers trims server entries and removes entries without a URL.
func NormalizeServers(servers []Server) []Server {
	if len(servers) == 0 {
		return nil
	}

	normalized := make([]Server, 0, len(servers))
	for _, server := range servers {
		server.URL = strings.TrimSpace(server.URL)
		server.Description = strings.TrimSpace(server.Description)
		if server.URL == "" {
			continue
		}
		normalized = append(normalized, server)
	}

	return normalized
}

// Paths maps OpenAPI path strings to method entries.
type Paths map[string]PathItem

// PathItem is the per-path OpenAPI method container.
type PathItem struct {
	// Get is the GET operation for the path.
	Get *Operation `json:"get,omitempty" yaml:"get,omitempty"`

	// Put is the PUT operation for the path.
	Put *Operation `json:"put,omitempty" yaml:"put,omitempty"`

	// Post is the POST operation for the path.
	Post *Operation `json:"post,omitempty" yaml:"post,omitempty"`

	// Delete is the DELETE operation for the path.
	Delete *Operation `json:"delete,omitempty" yaml:"delete,omitempty"`

	// Options is the OPTIONS operation for the path.
	Options *Operation `json:"options,omitempty" yaml:"options,omitempty"`

	// Head is the HEAD operation for the path.
	Head *Operation `json:"head,omitempty" yaml:"head,omitempty"`

	// Patch is the PATCH operation for the path.
	Patch *Operation `json:"patch,omitempty" yaml:"patch,omitempty"`

	// Trace is the TRACE operation for the path.
	Trace *Operation `json:"trace,omitempty" yaml:"trace,omitempty"`
}

// Operation is the minimal operation shape used by endpoint transcription.
type Operation struct {
	// OperationID is the stable machine-readable operation identifier.
	OperationID string `json:"operationId,omitempty" yaml:"operationId,omitempty"`

	// Summary is a short human-readable operation summary.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`

	// RequestBody is the OpenAPI 3 request body for this operation.
	RequestBody *RequestBody `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`

	// Parameters is the Swagger 2.0 operation parameter list.
	Parameters []Parameter `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	// Consumes is the Swagger 2.0 list of request media types.
	Consumes []string `json:"consumes,omitempty" yaml:"consumes,omitempty"`

	// Produces is the Swagger 2.0 list of response media types.
	Produces []string `json:"produces,omitempty" yaml:"produces,omitempty"`

	// Extensions stores OpenAPI Specification Extensions. MarshalJSON and
	// MarshalYAML emit these values inline.
	Extensions Extensions `json:"-" yaml:"-"`

	// Responses maps response status codes or default to response objects.
	Responses map[string]Response `json:"responses" yaml:"responses"`
}

// Response is the minimal OpenAPI response object.
type Response struct {
	// Description is the required human-readable response description.
	Description string `json:"description" yaml:"description"`

	// Content is the OpenAPI 3 response body content map.
	Content map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`

	// Schema is the Swagger 2.0 response body schema.
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// RequestBody is the OpenAPI 3 operation request body object.
type RequestBody struct {
	// Required reports whether the request body must be present.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// Content maps media types to payload descriptions.
	Content map[string]MediaType `json:"content" yaml:"content"`
}

// Parameter is a Swagger 2.0 operation parameter.
type Parameter struct {
	// In identifies where the parameter is supplied.
	In string `json:"in" yaml:"in"`

	// Name is the parameter name.
	Name string `json:"name" yaml:"name"`

	// Required reports whether the parameter must be supplied.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// Schema describes body parameters.
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// MediaType describes a request or response body for a single media type.
type MediaType struct {
	// Schema describes the JSON payload.
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

// Schema is a minimal provider-neutral OpenAPI schema object.
type Schema struct {
	// Type is the JSON Schema/OpenAPI type.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`

	// Format is the optional OpenAPI format.
	Format string `json:"format,omitempty" yaml:"format,omitempty"`

	// Minimum is the inclusive lower numeric bound.
	Minimum *int64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`

	// Maximum is the inclusive upper numeric bound.
	Maximum *int64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`

	// MinLength is the inclusive lower character-count bound for strings.
	MinLength *int64 `json:"minLength,omitempty" yaml:"minLength,omitempty"`

	// MaxLength is the inclusive upper character-count bound for strings.
	MaxLength *int64 `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`

	// MinProperties is the inclusive lower property-count bound for objects.
	MinProperties *int64 `json:"minProperties,omitempty" yaml:"minProperties,omitempty"`

	// MaxProperties is the inclusive upper property-count bound for objects.
	MaxProperties *int64 `json:"maxProperties,omitempty" yaml:"maxProperties,omitempty"`

	// MinItems is the inclusive lower item-count bound for arrays.
	MinItems *int64 `json:"minItems,omitempty" yaml:"minItems,omitempty"`

	// MaxItems is the inclusive upper item-count bound for arrays.
	MaxItems *int64 `json:"maxItems,omitempty" yaml:"maxItems,omitempty"`

	// Enum describes the allowed literal values for this schema.
	Enum []string `json:"enum,omitempty" yaml:"enum,omitempty"`

	// OneOf describes alternative schemas accepted at this position. OpenAPI
	// 3.x transcribers use it for discriminated request objects. Swagger 2.0
	// transcribers should downgrade these alternatives before emitting a
	// gateway document because Swagger 2.0 has no oneOf keyword.
	OneOf []Schema `json:"oneOf,omitempty" yaml:"oneOf,omitempty"`

	// AnyOf describes schemas where at least one alternative must match.
	AnyOf []Schema `json:"anyOf,omitempty" yaml:"anyOf,omitempty"`

	// AllOf describes schemas where every nested schema must match. Request
	// presence rules use this to layer object-level constraints over normal
	// object properties.
	AllOf []Schema `json:"allOf,omitempty" yaml:"allOf,omitempty"`

	// Not describes a schema that must not match. Swagger 2.0 transcribers
	// should omit it because Swagger 2.0 does not support not.
	Not *Schema `json:"not,omitempty" yaml:"not,omitempty"`

	// Discriminator identifies the property that selects one schema from OneOf.
	// It is primarily used by OpenAPI 3.x documents.
	Discriminator *Discriminator `json:"discriminator,omitempty" yaml:"discriminator,omitempty"`

	// Properties describes object properties.
	Properties map[string]Schema `json:"properties,omitempty" yaml:"properties,omitempty"`

	// Required lists required object property names.
	Required []string `json:"required,omitempty" yaml:"required,omitempty"`

	// AdditionalProperties describes values for arbitrary object keys.
	AdditionalProperties *Schema `json:"additionalProperties,omitempty" yaml:"additionalProperties,omitempty"`

	// Items describes array items.
	Items *Schema `json:"items,omitempty" yaml:"items,omitempty"`
}

// Discriminator is the OpenAPI 3 discriminator object.
type Discriminator struct {
	// PropertyName is the object property that selects the schema variant.
	PropertyName string `json:"propertyName" yaml:"propertyName"`

	// Mapping optionally maps discriminator values to schema references. The
	// current httpapi transcribers leave inline schemas unmapped.
	Mapping map[string]string `json:"mapping,omitempty" yaml:"mapping,omitempty"`
}

// Extensions contains OpenAPI Specification Extensions for an object.
//
// Extension names must begin with x-.
type Extensions map[string]any

// SetExtension sets an OpenAPI Specification Extension on the operation.
//
// Existing extensions with the same normalized name are replaced.
func (operation *Operation) SetExtension(name string, value any) error {
	name = normalizeExtensionName(name)
	if err := validateExtensionName(name); err != nil {
		return err
	}

	if operation.Extensions == nil {
		operation.Extensions = Extensions{}
	}
	for existing := range operation.Extensions {
		if normalizeExtensionName(existing) != name {
			continue
		}
		delete(operation.Extensions, existing)
	}
	operation.Extensions[name] = value

	return nil
}

// Extension returns one OpenAPI Specification Extension value.
func (operation Operation) Extension(name string) (any, bool) {
	name = normalizeExtensionName(name)
	if err := validateExtensionName(name); err != nil {
		return nil, false
	}

	for existing, value := range operation.Extensions {
		if normalizeExtensionName(existing) == name {
			return value, true
		}
	}

	return nil, false
}

// MarshalJSON emits operation extensions inline, as required by OpenAPI.
func (operation Operation) MarshalJSON() ([]byte, error) {
	object, err := operation.object()
	if err != nil {
		return nil, err
	}

	return json.Marshal(object)
}

// MarshalYAML emits operation extensions inline, as required by OpenAPI.
func (operation Operation) MarshalYAML() (any, error) {
	return operation.object()
}

// AddOperation adds an operation to the given path and method.
func (paths Paths) AddOperation(path string, method string, operation Operation) error {
	if paths == nil {
		return fmt.Errorf("openapi/spec: paths cannot be nil")
	}

	item := paths[path]
	if err := item.SetOperation(method, operation); err != nil {
		return fmt.Errorf("openapi/spec: add operation for %q: %w", path, err)
	}
	paths[path] = item

	return nil
}

// Merge copies path operations from other into paths.
func (paths Paths) Merge(other Paths) error {
	if paths == nil {
		return fmt.Errorf("openapi/spec: paths cannot be nil")
	}

	for path, item := range other {
		existing := paths[path]
		if err := existing.Merge(item); err != nil {
			return fmt.Errorf("openapi/spec: merge path %q: %w", path, err)
		}
		paths[path] = existing
	}

	return nil
}

// SetOperation sets one method operation on a path item.
func (p *PathItem) SetOperation(method string, operation Operation) error {
	slot, err := p.operationSlot(method)
	if err != nil {
		return err
	}
	if *slot != nil {
		return fmt.Errorf("duplicate %s operation", normalizeMethod(method))
	}

	*slot = &operation
	return nil
}

// Merge adds all operations from other into p.
func (p *PathItem) Merge(other PathItem) error {
	operations := []struct {
		method    string
		operation *Operation
	}{
		{method: "GET", operation: other.Get},
		{method: "PUT", operation: other.Put},
		{method: "POST", operation: other.Post},
		{method: "DELETE", operation: other.Delete},
		{method: "OPTIONS", operation: other.Options},
		{method: "HEAD", operation: other.Head},
		{method: "PATCH", operation: other.Patch},
		{method: "TRACE", operation: other.Trace},
	}

	for _, operation := range operations {
		if operation.operation == nil {
			continue
		}
		if err := p.SetOperation(operation.method, *operation.operation); err != nil {
			return err
		}
	}

	return nil
}

func (operation Operation) object() (map[string]any, error) {
	object := map[string]any{
		"responses": operation.responses(),
	}
	if operation.OperationID != "" {
		object["operationId"] = operation.OperationID
	}
	if operation.Summary != "" {
		object["summary"] = operation.Summary
	}
	if operation.RequestBody != nil {
		object["requestBody"] = operation.RequestBody
	}
	if len(operation.Parameters) > 0 {
		object["parameters"] = operation.Parameters
	}
	if len(operation.Consumes) > 0 {
		object["consumes"] = operation.Consumes
	}
	if len(operation.Produces) > 0 {
		object["produces"] = operation.Produces
	}

	extensions, err := normalizeExtensions(operation.Extensions)
	if err != nil {
		return nil, err
	}
	maps.Copy(object, extensions)

	return object, nil
}

func (operation Operation) responses() map[string]Response {
	if operation.Responses == nil {
		return map[string]Response{}
	}

	return operation.Responses
}

func (p *PathItem) operationSlot(method string) (**Operation, error) {
	if p == nil {
		return nil, fmt.Errorf("openapi/spec: path item cannot be nil")
	}

	switch normalizeMethod(method) {
	case "GET":
		return &p.Get, nil
	case "PUT":
		return &p.Put, nil
	case "POST":
		return &p.Post, nil
	case "DELETE":
		return &p.Delete, nil
	case "OPTIONS":
		return &p.Options, nil
	case "HEAD":
		return &p.Head, nil
	case "PATCH":
		return &p.Patch, nil
	case "TRACE":
		return &p.Trace, nil
	default:
		return nil, fmt.Errorf("unsupported openapi method %q", method)
	}
}

func normalizeMethod(method string) string {
	return strings.ToUpper(strings.TrimSpace(method))
}

func normalizeExtensions(extensions Extensions) (Extensions, error) {
	if len(extensions) == 0 {
		return nil, nil
	}

	normalized := make(Extensions, len(extensions))
	for rawName, value := range extensions {
		name := normalizeExtensionName(rawName)
		if err := validateExtensionName(name); err != nil {
			return nil, err
		}
		if _, exists := normalized[name]; exists {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateExtensionName, name)
		}
		normalized[name] = value
	}

	return normalized, nil
}

func normalizeExtensionName(name string) string {
	return strings.TrimSpace(name)
}

func validateExtensionName(name string) error {
	if !strings.HasPrefix(name, "x-") || name == "x-" {
		return fmt.Errorf("%w: %q", ErrInvalidExtensionName, name)
	}

	return nil
}
