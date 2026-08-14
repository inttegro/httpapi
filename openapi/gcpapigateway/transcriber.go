package gcpapigateway

import (
	"errors"
	"fmt"
	"strings"
	"time"

	endpointpkg "github.com/zebodotdev/httpapi/endpoint"
	internalroute "github.com/zebodotdev/httpapi/openapi/internal/route"
	internalschema "github.com/zebodotdev/httpapi/openapi/internal/schema"
	"github.com/zebodotdev/httpapi/openapi/spec"
)

const (
	// DocumentSpecVersion is the Swagger version emitted for GCP API Gateway.
	DocumentSpecVersion = "2.0"

	// DefaultDocumentTitle is used when Transcriber.Title is empty.
	DefaultDocumentTitle = "http api"

	// DefaultDocumentDescription is used when Transcriber.Description is empty.
	DefaultDocumentDescription = "machine-readable representation of the http api"

	// DefaultScheme is the Swagger schemes value emitted by this transcriber.
	DefaultScheme = "https"

	// BackendExtensionName is the GCP API Gateway backend extension field.
	BackendExtensionName = "x-google-backend"

	// HTTPAPIInternalExtensionName records httpapi internal-only metadata on
	// generated gateway operations.
	HTTPAPIInternalExtensionName = "x-httpapi-internal"

	// HTTPAPIAuthorizationExtensionName records httpapi endpoint authorization
	// metadata on generated gateway operations.
	HTTPAPIAuthorizationExtensionName = "x-httpapi-authorization"

	// HTTPAPIPriorityExtensionName records endpoint priority metadata on
	// generated gateway operations.
	HTTPAPIPriorityExtensionName = "x-httpapi-priority"

	// PathTranslationAppend is the GCP backend mode for appending the matched
	// request path to the backend address.
	PathTranslationAppend = "APPEND_PATH_TO_ADDRESS"

	// PathTranslationConstant is the GCP backend mode for treating the backend
	// address as the complete upstream target.
	PathTranslationConstant = "CONSTANT_ADDRESS"

	// BackendDeadlineMax is the maximum backend deadline accepted by GCP API
	// Gateway.
	BackendDeadlineMax = 600 * time.Second

	// PlaceholderResponseDescription is used because Swagger requires at least
	// one response entry per operation.
	PlaceholderResponseDescription = "Required placeholder response."
)

var (
	// ErrDocumentVersionRequired reports a document transcription request
	// without a version.
	ErrDocumentVersionRequired = errors.New(
		"gcpapigateway: document version is required",
	)

	// ErrBackendAddressRequired reports an endpoint route that cannot resolve a
	// backend address.
	ErrBackendAddressRequired = errors.New(
		"gcpapigateway: backend address is required",
	)

	// ErrBackendDeadlineExceeded reports a route backend timeout greater than
	// BackendDeadlineMax.
	ErrBackendDeadlineExceeded = errors.New(
		"gcpapigateway: backend deadline exceeds maximum",
	)
)

// Transcriber writes routes into a GCP API Gateway Swagger 2.0 shape.
type Transcriber struct {
	// PathPrefix is prepended to every transcribed path.
	PathPrefix string

	// Info is the generated Swagger info block. Info.Version is required for
	// document transcription. Empty title and description values use documented
	// httpapi defaults so generated gateway documents stay valid during early
	// integration.
	Info spec.Info

	// Host is the Swagger 2.0 host value for the API Gateway document.
	Host string

	// DefaultBackend is the provider-neutral fallback backend for routes that do
	// not define RouteSpec.Backend fields. This transcriber translates the
	// resolved backend into GCP API Gateway's x-google-backend extension.
	DefaultBackend endpointpkg.RouteBackend
}

// Backend is the GCP API Gateway backend extension payload.
type Backend struct {
	// Address is the upstream backend address for this operation.
	Address string `json:"address,omitempty" yaml:"address,omitempty"`

	// PathTranslation is the GCP API Gateway path translation mode.
	PathTranslation string `json:"path_translation,omitempty" yaml:"path_translation,omitempty"`

	// Deadline is the optional backend deadline in seconds.
	Deadline *float64 `json:"deadline,omitempty" yaml:"deadline,omitempty"`
}

// TranscribeEndpoint emits GCP API Gateway path entries for one endpoint.
//
// It returns only the paths map, which is useful when composing a larger
// document manually.
func (t Transcriber) TranscribeEndpoint(endpoint endpointpkg.Endpoint) (spec.Paths, error) {
	routes, err := internalroute.FromEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	return t.transcribe(routes)
}

// TranscribeGroup emits GCP API Gateway path entries for one endpoint group.
func (t Transcriber) TranscribeGroup(group endpointpkg.EndpointGroup) (spec.Paths, error) {
	routes, err := internalroute.FromGroup(group)
	if err != nil {
		return nil, err
	}

	return t.transcribe(routes)
}

// TranscribeGroups emits GCP API Gateway path entries for several endpoint
// groups.
func (t Transcriber) TranscribeGroups(groups ...endpointpkg.EndpointGroup) (spec.Paths, error) {
	routes, err := internalroute.FromGroups(groups...)
	if err != nil {
		return nil, err
	}

	return t.transcribe(routes)
}

// TranscribeEndpointDocument emits a complete GCP API Gateway document for one
// endpoint.
func (t Transcriber) TranscribeEndpointDocument(endpoint endpointpkg.Endpoint) (spec.Document, error) {
	routes, err := internalroute.FromEndpoint(endpoint)
	if err != nil {
		return spec.Document{}, err
	}

	return t.transcribeDocument(routes)
}

// TranscribeGroupDocument emits a GCP API Gateway document for one endpoint group.
func (t Transcriber) TranscribeGroupDocument(group endpointpkg.EndpointGroup) (spec.Document, error) {
	routes, err := internalroute.FromGroup(group)
	if err != nil {
		return spec.Document{}, err
	}

	return t.transcribeDocument(routes)
}

// TranscribeGroupsDocument emits a GCP API Gateway document for several endpoint
// groups.
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
		operation, err := t.operationForRoute(route)
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

	return spec.Document{
		Swagger:  DocumentSpecVersion,
		Info:     info,
		Host:     strings.TrimSpace(t.Host),
		Schemes:  []string{DefaultScheme},
		Produces: []string{endpointpkg.ApplicationJson},
		Paths:    paths,
	}, nil
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

func (t Transcriber) operationForRoute(route internalroute.Route) (spec.Operation, error) {
	routeSpec := route.Endpoint.RouteSpec()
	operationSpec := route.Endpoint.Operation()
	backend, err := t.gatewayBackend(routeSpec.Backend)
	if err != nil {
		return spec.Operation{}, err
	}

	operation := spec.Operation{
		OperationID: operationIDForRoute(route),
		Summary:     fmt.Sprintf("%s %s", route.Method, route.Path),
		Parameters:  parametersForEndpoint(route.Endpoint),
		Consumes:    contentTypesForOpenAPI(route.Endpoint.AcceptedContentTypes()),
		Produces:    []string{endpointpkg.ApplicationJson},
		Responses:   responsesForEndpoint(route.Endpoint),
	}
	if err := operation.SetExtension(BackendExtensionName, backend); err != nil {
		return spec.Operation{}, err
	}
	if operationSpec.Summary != "" {
		operation.Summary = operationSpec.Summary
	}
	if route.Endpoint.IsInternal() {
		if err := operation.SetExtension(HTTPAPIInternalExtensionName, true); err != nil {
			return spec.Operation{}, err
		}
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
			"gcpapigateway: duplicate operation id %q for %s %s and %s %s",
			operationID,
			existing.Method, existing.Path,
			route.Method, route.Path,
		)
	}
	seen[operationID] = route
	return nil
}

func (t Transcriber) gatewayBackend(backend endpointpkg.RouteBackend) (Backend, error) {
	backend = backend.WithDefaults(t.DefaultBackend.WithDefaults(endpointpkg.RouteBackend{
		PathMode: endpointpkg.RoutePathModeAppend,
	}))
	backend = endpointpkg.NormalizeRouteBackend(backend)
	if backend.Address == "" {
		return Backend{}, ErrBackendAddressRequired
	}

	pathTranslation := PathTranslationAppend
	switch backend.PathMode {
	case "", endpointpkg.RoutePathModeAppend:
		pathTranslation = PathTranslationAppend
	case endpointpkg.RoutePathModeConstant:
		pathTranslation = PathTranslationConstant
	default:
		return Backend{}, fmt.Errorf(
			"gcpapigateway: unsupported route path mode %q",
			backend.PathMode,
		)
	}

	deadline, err := gatewayBackendDeadline(backend)
	if err != nil {
		return Backend{}, err
	}

	return Backend{
		Address:         backend.Address,
		PathTranslation: pathTranslation,
		Deadline:        deadline,
	}, nil
}

func gatewayBackendDeadline(backend endpointpkg.RouteBackend) (*float64, error) {
	if backend.Timeout == 0 {
		return nil, nil
	}
	if backend.Timeout > BackendDeadlineMax {
		return nil, fmt.Errorf(
			"%w: timeout=%s max=%s",
			ErrBackendDeadlineExceeded,
			backend.Timeout,
			BackendDeadlineMax,
		)
	}

	seconds := backend.Timeout.Seconds()
	return &seconds, nil
}

func contentTypesForOpenAPI(contentTypes []endpointpkg.ContentType) []string {
	contentTypes = endpointpkg.NormalizeContentTypeSlice(contentTypes)
	values := make([]string, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		values = append(values, string(contentType))
	}
	return values
}

func placeholderResponses() map[string]spec.Response {
	return map[string]spec.Response{
		"default": {Description: PlaceholderResponseDescription},
	}
}

func parametersForEndpoint(endpoint endpointpkg.Endpoint) []spec.Parameter {
	contract := endpoint.RequestContract()
	if contract.Body.Type == "" {
		return nil
	}

	schema := internalschema.FromParamShapeSwagger2(contract.Body)
	return []spec.Parameter{
		{
			In:       "body",
			Name:     "body",
			Required: contract.Required,
			Schema:   &schema,
		},
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
		schema := internalschema.FromResponseShape(contract.Body)
		response.Schema = &schema
	}
	return response
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
