package openapi31

import (
	"errors"
	"fmt"
	"strings"

	endpointpkg "github.com/zebodotdev/httpapi/endpoint"
	internalroute "github.com/zebodotdev/httpapi/openapi/internal/route"
	internalschema "github.com/zebodotdev/httpapi/openapi/internal/schema"
	"github.com/zebodotdev/httpapi/openapi/spec"
)

const (
	// DocumentSpecVersion is the OpenAPI version emitted by this transcriber.
	DocumentSpecVersion = "3.1.1"

	// DefaultDocumentTitle is used when Transcriber.Title is empty.
	DefaultDocumentTitle = "http api"

	// DefaultDocumentDescription is used when Transcriber.Description is empty.
	DefaultDocumentDescription = "machine-readable representation of the http api"

	// PlaceholderResponseDescription is used because OpenAPI requires at least
	// one response entry per operation.
	PlaceholderResponseDescription = "Required placeholder response."

	// HTTPAPIAuthorizationExtensionName records httpapi endpoint authorization
	// metadata on generated public operations.
	HTTPAPIAuthorizationExtensionName = "x-httpapi-authorization"

	// HTTPAPIPriorityExtensionName records endpoint priority metadata on
	// generated public operations.
	HTTPAPIPriorityExtensionName = "x-httpapi-priority"
)

// ErrDocumentVersionRequired reports a document transcription request without a
// version.
var ErrDocumentVersionRequired = errors.New(
	"openapi31: document version is required",
)

// Transcriber writes routes into a public OpenAPI 3.1 shape.
type Transcriber struct {
	// PathPrefix is prepended to every transcribed path.
	PathPrefix string

	// Info is the generated OpenAPI info block. Info.Version is required for
	// document transcription. Empty title and description values use documented
	// httpapi defaults so quick prototypes still emit valid OpenAPI.
	Info spec.Info

	// Servers lists the public base URLs clients can use for this API.
	Servers []spec.Server
}

// TranscribeEndpoint emits public OpenAPI path entries for one endpoint.
//
// Internal endpoints are omitted from public OpenAPI output.
func (t Transcriber) TranscribeEndpoint(endpoint endpointpkg.Endpoint) (spec.Paths, error) {
	routes, err := internalroute.FromEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	return t.transcribe(routes)
}

// TranscribeGroup emits public OpenAPI path entries for one endpoint group.
//
// Internal endpoints are omitted from public OpenAPI output.
func (t Transcriber) TranscribeGroup(group endpointpkg.EndpointGroup) (spec.Paths, error) {
	routes, err := internalroute.FromGroup(group)
	if err != nil {
		return nil, err
	}

	return t.transcribe(routes)
}

// TranscribeGroups emits public OpenAPI path entries for several endpoint
// groups.
//
// Internal endpoints are omitted from public OpenAPI output.
func (t Transcriber) TranscribeGroups(groups ...endpointpkg.EndpointGroup) (spec.Paths, error) {
	routes, err := internalroute.FromGroups(groups...)
	if err != nil {
		return nil, err
	}

	return t.transcribe(routes)
}

// TranscribeEndpointDocument emits a public OpenAPI 3.1 document for one
// endpoint.
func (t Transcriber) TranscribeEndpointDocument(endpoint endpointpkg.Endpoint) (spec.Document, error) {
	routes, err := internalroute.FromEndpoint(endpoint)
	if err != nil {
		return spec.Document{}, err
	}

	return t.transcribeDocument(routes)
}

// TranscribeGroupDocument emits a public OpenAPI 3.1 document for one endpoint group.
func (t Transcriber) TranscribeGroupDocument(group endpointpkg.EndpointGroup) (spec.Document, error) {
	routes, err := internalroute.FromGroup(group)
	if err != nil {
		return spec.Document{}, err
	}

	return t.transcribeDocument(routes)
}

// TranscribeGroupsDocument emits a public OpenAPI 3.1 document for several
// endpoint groups.
func (t Transcriber) TranscribeGroupsDocument(groups ...endpointpkg.EndpointGroup) (spec.Document, error) {
	routes, err := internalroute.FromGroups(groups...)
	if err != nil {
		return spec.Document{}, err
	}

	return t.transcribeDocument(routes)
}

func (t Transcriber) transcribe(routes internalroute.Routes) (spec.Paths, error) {
	routes, err := t.routes(routes)
	if err != nil {
		return nil, err
	}

	paths := spec.Paths{}
	operationIDs := map[string]internalroute.Route{}
	for _, route := range routes {
		if route.Endpoint.IsInternal() {
			continue
		}
		operation, err := operationForRoute(route)
		if err != nil {
			return nil, err
		}
		if err := ensureUniqueOperationID(operationIDs, operation.OperationID, route); err != nil {
			return nil, err
		}
		if err := paths.AddOperation(route.Path, route.Method, operation); err != nil {
			return nil, err
		}
	}

	return paths, nil
}

func (t Transcriber) transcribeDocument(routes internalroute.Routes) (spec.Document, error) {
	info := t.documentInfo()
	if strings.TrimSpace(info.Version) == "" {
		return spec.Document{}, ErrDocumentVersionRequired
	}

	paths, err := t.transcribe(routes)
	if err != nil {
		return spec.Document{}, err
	}

	doc := spec.Document{
		OpenAPI: DocumentSpecVersion,
		Info:    info,
		Servers: t.documentServers(),
		Paths:   paths,
	}

	return doc, nil
}

func (t Transcriber) routes(routes internalroute.Routes) (internalroute.Routes, error) {
	if strings.TrimSpace(t.PathPrefix) == "" {
		return routes, nil
	}

	return routes.WithPathPrefix(t.PathPrefix)
}

func (t Transcriber) documentInfo() spec.Info {
	info := spec.NormalizeInfo(t.Info)
	if info.Title == "" {
		info.Title = DefaultDocumentTitle
	}
	if info.Description == "" {
		info.Description = DefaultDocumentDescription
	}

	return info
}

func (t Transcriber) documentServers() []spec.Server {
	return spec.NormalizeServers(t.Servers)
}

func operationForRoute(route internalroute.Route) (spec.Operation, error) {
	operationSpec := route.Endpoint.Operation()
	operation := spec.Operation{
		OperationID: operationIDForRoute(route),
		Summary:     fmt.Sprintf("%s %s", route.Method, route.Path),
		RequestBody: requestBodyForEndpoint(route.Endpoint),
		Responses:   responsesForEndpoint(route.Endpoint),
	}
	if operationSpec.Summary != "" {
		operation.Summary = operationSpec.Summary
	}
	if auth := route.Endpoint.Authorization(); auth.Required {
		if err := operation.SetExtension(HTTPAPIAuthorizationExtensionName, auth); err != nil {
			return spec.Operation{}, err
		}
	}
	if priority := route.Endpoint.Priority(); priority != "" {
		if err := operation.SetExtension(HTTPAPIPriorityExtensionName, priority); err != nil {
			return spec.Operation{}, err
		}
	}

	return operation, nil
}

func operationIDForRoute(route internalroute.Route) string {
	operationSpec := route.Endpoint.Operation()
	if route.Alias == nil {
		if operationSpec.ID != "" {
			return operationSpec.ID
		}
		return defaultOperationID(route.Method, route.Path)
	}
	if route.Alias.OperationID != "" {
		return route.Alias.OperationID
	}

	canonicalID := operationSpec.ID
	if canonicalID == "" {
		canonicalPath := route.CanonicalPath
		if canonicalPath == "" {
			canonicalPath = route.Path
		}
		canonicalID = defaultOperationID(route.Method, canonicalPath)
	}

	return canonicalID + "Alias"
}

func ensureUniqueOperationID(
	seen map[string]internalroute.Route,
	operationID string,
	route internalroute.Route,
) error {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil
	}
	if existing, ok := seen[operationID]; ok {
		return fmt.Errorf(
			"openapi31: duplicate operation id %q for %s %s and %s %s",
			operationID,
			existing.Method, existing.Path,
			route.Method, route.Path,
		)
	}
	seen[operationID] = route
	return nil
}

func placeholderResponses() map[string]spec.Response {
	return map[string]spec.Response{
		"default": {Description: PlaceholderResponseDescription},
	}
}

func requestBodyForEndpoint(endpoint endpointpkg.Endpoint) *spec.RequestBody {
	contract := endpoint.RequestContract()
	if contract.Body.Type == "" {
		return nil
	}

	schema := internalschema.FromParamShape(contract.Body)
	return &spec.RequestBody{
		Required: contract.Required,
		Content:  mediaTypesForSchema(contentTypesForOpenAPI(endpoint.AcceptedContentTypes()), schema),
	}
}

func responsesForEndpoint(endpoint endpointpkg.Endpoint) map[string]spec.Response {
	contracts := endpoint.ResponseContracts()
	if len(contracts) == 0 {
		return placeholderResponses()
	}

	responses := make(map[string]spec.Response, len(contracts))
	for _, contract := range contracts {
		responses[responseStatusCode(contract.Status)] = responseForContract(contract)
	}
	return responses
}

func responseForContract(contract endpointpkg.ResponseContract) spec.Response {
	response := spec.Response{Description: responseDescription(contract.Description)}
	if contract.Body.Type != "" {
		response.Content = mediaTypesForSchema(
			[]string{responseContentType(contract.ContentType)},
			internalschema.FromResponseShape(contract.Body),
		)
	}
	return response
}

func mediaTypesForSchema(contentTypes []string, schema spec.Schema) map[string]spec.MediaType {
	content := make(map[string]spec.MediaType, len(contentTypes))
	for _, contentType := range contentTypes {
		schemaCopy := schema
		content[contentType] = spec.MediaType{Schema: &schemaCopy}
	}
	return content
}

func contentTypesForOpenAPI(contentTypes []endpointpkg.ContentType) []string {
	contentTypes = endpointpkg.NormalizeContentTypeSlice(contentTypes)
	values := make([]string, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		values = append(values, string(contentType))
	}
	return values
}

func responseStatusCode(status int) string {
	if status == 0 {
		return "default"
	}
	return fmt.Sprintf("%d", status)
}

func responseDescription(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return "Response."
	}
	return description
}

func responseContentType(contentType endpointpkg.ContentType) string {
	if contentType == "" {
		contentType = endpointpkg.ApplicationJson
	}
	return string(endpointpkg.NormalizeContentType(contentType))
}

func defaultOperationID(method endpointpkg.HttpMethod, path string) string {
	parts := []string{strings.ToLower(method)}
	for _, part := range strings.FieldsFunc(path, operationIDSeparator) {
		part = strings.Trim(part, "{}")
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}

	return strings.Join(parts, "_")
}

func operationIDSeparator(r rune) bool {
	return r == '/' || r == '-' || r == '_' || r == '.' || r == ':'
}
