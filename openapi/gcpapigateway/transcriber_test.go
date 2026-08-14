package gcpapigateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	endpointpkg "github.com/zebodotdev/httpapi/endpoint"
	"github.com/zebodotdev/httpapi/openapi/spec"
	"github.com/zebodotdev/httpapi/param"
	"github.com/zebodotdev/httpapi/response"
)

func TestTranscribeEmitsBackendAndDefaultResponse(t *testing.T) {
	paths, err := Transcriber{
		DefaultBackend: endpointpkg.RouteBackend{
			Address: "https://service.example.internal",
		},
	}.TranscribeEndpoint(endpointpkg.NewEndpoint(
		endpointpkg.POST,
		"/internal/sync",
		noopGCPGatewayHandler,
		endpointpkg.WithInternal(),
		endpointpkg.WithRequiredAuthorization(endpointpkg.AuthorizationKindService),
	))
	if err != nil {
		t.Fatalf("TranscribeEndpoint() error = %v", err)
	}

	operation := paths["/internal/sync"].Post
	if operation == nil {
		t.Fatal("post operation missing")
	}
	backend := operationBackend(t, *operation)
	if backend.Address != "https://service.example.internal" {
		t.Fatalf("backend address = %q", backend.Address)
	}
	if backend.PathTranslation != PathTranslationAppend {
		t.Fatalf("path translation = %q", backend.PathTranslation)
	}
	if len(operation.Consumes) != 1 || operation.Consumes[0] != endpointpkg.ApplicationJson {
		t.Fatalf("consumes = %#v, want application/json", operation.Consumes)
	}
	if len(operation.Produces) != 1 || operation.Produces[0] != endpointpkg.ApplicationJson {
		t.Fatalf("produces = %#v, want application/json", operation.Produces)
	}
	if operation.Responses["default"].Description != PlaceholderResponseDescription {
		t.Fatalf("default response = %#v", operation.Responses["default"])
	}
	internal, ok := operation.Extension(HTTPAPIInternalExtensionName)
	if !ok || internal != true {
		t.Fatalf("internal metadata = %#v, want true", internal)
	}
	authValue, ok := operation.Extension(HTTPAPIAuthorizationExtensionName)
	if !ok {
		t.Fatal("authorization metadata missing")
	}
	auth, ok := authValue.(endpointpkg.AuthorizationRequirement)
	if !ok || auth.Kind != endpointpkg.AuthorizationKindService {
		t.Fatalf("authorization metadata = %#v", authValue)
	}

	encoded, err := json.Marshal(paths)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"x-google-backend"`) {
		t.Fatalf("encoded gateway path does not contain x-google-backend: %s", encoded)
	}
}

func TestTranscribeUsesRouteSpecBackendAndGroupDefaults(t *testing.T) {
	group := endpointpkg.EndpointGroup{PathPrefix: "/orders"}
	group.Add(endpointpkg.NewEndpoint(endpointpkg.POST, "/new", noopGCPGatewayHandler))
	group.Add(endpointpkg.NewEndpoint(
		endpointpkg.POST,
		"/lookup",
		noopGCPGatewayHandler,
		endpointpkg.WithOperationSpec(endpointpkg.OperationSpec{
			ID: "lookup_order",
		}),
		endpointpkg.WithRouteSpec(endpointpkg.RouteSpec{
			Backend: endpointpkg.RouteBackend{
				Address:  "https://lookup.example.internal",
				PathMode: endpointpkg.RoutePathModeConstant,
				Timeout:  45 * time.Second,
			},
		}),
	))

	group.ConfigureOperationSpec(endpointpkg.OperationSpec{
		ID:      "orders_group",
		Summary: "Orders endpoint",
	})
	group.ConfigureRouteBackend(endpointpkg.RouteBackend{
		Address: "https://tasks.example.internal",
		Timeout: 20 * time.Second,
	})

	paths, err := Transcriber{
		DefaultBackend: endpointpkg.RouteBackend{
			Address: "https://fallback.example.internal",
		},
	}.TranscribeGroup(group)
	if err != nil {
		t.Fatalf("TranscribeGroup() error = %v", err)
	}

	createOperation := paths["/orders/new"].Post
	if createOperation == nil {
		t.Fatal("create operation missing")
	}
	if createOperation.Summary != "Orders endpoint" {
		t.Fatalf("create summary = %q", createOperation.Summary)
	}
	if createOperation.OperationID == "orders_group" {
		t.Fatal("group operation id was inherited by endpoint")
	}
	createBackend := operationBackend(t, *createOperation)
	if createBackend.Address != "https://tasks.example.internal" {
		t.Fatalf("create backend = %#v", createBackend)
	}
	if createBackend.PathTranslation != PathTranslationAppend {
		t.Fatalf("create path translation = %q", createBackend.PathTranslation)
	}
	if createBackend.Deadline == nil ||
		*createBackend.Deadline != 20 {
		t.Fatalf("create deadline = %#v, want 20", createBackend.Deadline)
	}

	lookupOperation := paths["/orders/lookup"].Post
	if lookupOperation == nil {
		t.Fatal("lookup operation missing")
	}
	if lookupOperation.OperationID != "lookup_order" {
		t.Fatalf("lookup operation id = %q", lookupOperation.OperationID)
	}
	if lookupOperation.Summary != "Orders endpoint" {
		t.Fatalf("lookup summary = %q", lookupOperation.Summary)
	}
	lookupBackend := operationBackend(t, *lookupOperation)
	if lookupBackend.Address != "https://lookup.example.internal" {
		t.Fatalf("lookup backend = %#v", lookupBackend)
	}
	if lookupBackend.PathTranslation != PathTranslationConstant {
		t.Fatalf("lookup path translation = %q", lookupBackend.PathTranslation)
	}
	if lookupBackend.Deadline == nil ||
		*lookupBackend.Deadline != 45 {
		t.Fatalf("lookup deadline = %#v, want 45", lookupBackend.Deadline)
	}
}

func TestTranscribeEndpointAliasesUseSameBackendAndRouteOperationIDs(t *testing.T) {
	paths, err := Transcriber{
		DefaultBackend: endpointpkg.RouteBackend{
			Address: "https://service.example.internal",
		},
	}.TranscribeEndpoint(endpointpkg.DefineEndpoint(endpointpkg.EndpointSpec{
		Method: endpointpkg.POST,
		Path:   "/orders/create",
		Aliases: []endpointpkg.AliasSpec{
			{Path: "/orders/new"},
		},
		Handler: noopGCPGatewayHandler,
		Operation: endpointpkg.OperationSpec{
			ID:      "createOrder",
			Summary: "Create order",
		},
	}))
	if err != nil {
		t.Fatalf("TranscribeEndpoint() error = %v", err)
	}

	canonical := paths["/orders/create"].Post
	if canonical == nil {
		t.Fatal("canonical operation missing")
	}
	if canonical.OperationID != "createOrder" {
		t.Fatalf("canonical operation id = %q, want createOrder", canonical.OperationID)
	}

	alias := paths["/orders/new"].Post
	if alias == nil {
		t.Fatal("alias operation missing")
	}
	if alias.OperationID != "createOrderAlias" {
		t.Fatalf("alias operation id = %q, want createOrderAlias", alias.OperationID)
	}
	if alias.Summary != "Create order" {
		t.Fatalf("alias summary = %q, want Create order", alias.Summary)
	}
	if operationBackend(t, *alias).Address != "https://service.example.internal" {
		t.Fatalf("alias backend = %#v", operationBackend(t, *alias))
	}
}

func TestTranscribeRejectsBackendTimeoutAboveGatewayLimit(t *testing.T) {
	_, err := Transcriber{}.TranscribeEndpoint(endpointpkg.NewEndpoint(
		endpointpkg.POST,
		"/exports/start",
		noopGCPGatewayHandler,
		endpointpkg.WithRouteBackend(endpointpkg.RouteBackend{
			Address: "https://service.example.internal",
			Timeout: BackendDeadlineMax + time.Second,
		}),
	))
	if !errors.Is(err, ErrBackendDeadlineExceeded) {
		t.Fatalf("error = %v, want ErrBackendDeadlineExceeded", err)
	}
}

func TestTranscribeDocumentIncludesSwaggerShape(t *testing.T) {
	doc, err := Transcriber{
		Info: spec.Info{
			Version: "0.1.0-beta5",
		},
		Host: "api.example.gateway.dev",
		DefaultBackend: endpointpkg.RouteBackend{
			Address: "https://service.example.internal",
		},
	}.TranscribeEndpointDocument(
		endpointpkg.NewEndpoint(endpointpkg.POST, "/orders/new", noopGCPGatewayHandler),
	)
	if err != nil {
		t.Fatalf("TranscribeEndpointDocument() error = %v", err)
	}

	if doc.Swagger != DocumentSpecVersion {
		t.Fatalf("swagger = %q, want %q", doc.Swagger, DocumentSpecVersion)
	}
	if doc.Info.Version != "0.1.0-beta5" {
		t.Fatalf("info.version = %q", doc.Info.Version)
	}
	if doc.Host != "api.example.gateway.dev" {
		t.Fatalf("host = %q", doc.Host)
	}
	if len(doc.Schemes) != 1 || doc.Schemes[0] != DefaultScheme {
		t.Fatalf("schemes = %#v", doc.Schemes)
	}
	if len(doc.Produces) != 1 || doc.Produces[0] != endpointpkg.ApplicationJson {
		t.Fatalf("produces = %#v", doc.Produces)
	}
	if operationBackend(t, *doc.Paths["/orders/new"].Post) == (Backend{}) {
		t.Fatal("gateway backend missing from document operation")
	}
}

func TestTranscribeDocumentRequiresVersion(t *testing.T) {
	_, err := Transcriber{
		DefaultBackend: endpointpkg.RouteBackend{
			Address: "https://service.example.internal",
		},
	}.TranscribeEndpointDocument(
		endpointpkg.NewEndpoint(endpointpkg.POST, "/orders/new", noopGCPGatewayHandler),
	)
	if err != ErrDocumentVersionRequired {
		t.Fatalf("error = %v, want ErrDocumentVersionRequired", err)
	}
}

func TestTranscribeUsesEndpointRequestAndResponseContracts(t *testing.T) {
	paths, err := Transcriber{
		DefaultBackend: endpointpkg.RouteBackend{
			Address: "https://service.example.internal",
		},
	}.TranscribeEndpoint(endpointpkg.DefineEndpoint(endpointpkg.EndpointSpec{
		Method:    endpointpkg.POST,
		Path:      "/orders/new",
		Handler:   noopGCPGatewayHandler,
		Request:   endpointpkg.RequestBody(gatewayOrderRequestParser()),
		Responses: []endpointpkg.ResponseContract{endpointpkg.ResponseBody(http.StatusCreated, "Created order.", gatewayOrderResponseShape())},
	}))
	if err != nil {
		t.Fatalf("TranscribeEndpoint() error = %v", err)
	}

	operation := paths["/orders/new"].Post
	if operation == nil {
		t.Fatal("post operation missing")
	}
	if len(operation.Parameters) != 1 {
		t.Fatalf("parameters = %#v, want one body parameter", operation.Parameters)
	}
	bodyParam := operation.Parameters[0]
	if bodyParam.In != "body" || bodyParam.Name != "body" || !bodyParam.Required {
		t.Fatalf("body parameter = %#v", bodyParam)
	}
	if bodyParam.Schema == nil {
		t.Fatal("body parameter schema missing")
	}
	if bodyParam.Schema.Properties["order_id"].Type != "string" {
		t.Fatalf("order_id schema = %#v", bodyParam.Schema.Properties["order_id"])
	}
	if len(bodyParam.Schema.Required) != 1 || bodyParam.Schema.Required[0] != "order_id" {
		t.Fatalf("request required = %#v", bodyParam.Schema.Required)
	}

	created := operation.Responses["201"]
	if created.Description != "Created order." {
		t.Fatalf("created response = %#v", created)
	}
	if created.Schema == nil {
		t.Fatal("created response schema missing")
	}
	if created.Schema.Properties["id"].Type != "string" {
		t.Fatalf("id response schema = %#v", created.Schema.Properties["id"])
	}
	if created.Schema.Properties["count"].Format != "int64" {
		t.Fatalf("count response schema = %#v", created.Schema.Properties["count"])
	}
}

func noopGCPGatewayHandler(*endpointpkg.Req) {}

func operationBackend(t *testing.T, operation spec.Operation) Backend {
	t.Helper()

	value, ok := operation.Extension(BackendExtensionName)
	if !ok {
		t.Fatal("x-google-backend missing")
	}
	backend, ok := value.(Backend)
	if !ok {
		t.Fatalf("x-google-backend type = %T, want Backend", value)
	}

	return backend
}

type gatewayOrderRequest struct {
	OrderID string
}

type gatewayOrderResponse struct {
	ID    string
	Count int64
}

func gatewayOrderRequestParser() *param.Request[gatewayOrderRequest] {
	return param.JSON[gatewayOrderRequest]().
		Param(param.Required("order_id", param.String())).
		Parse(func(values param.Values) (gatewayOrderRequest, error) {
			return gatewayOrderRequest{
				OrderID: param.Must[string](values, "order_id"),
			}, nil
		})
}

func gatewayOrderResponseShape() response.Shape[gatewayOrderResponse] {
	return response.Object[gatewayOrderResponse](
		response.Required("id", response.String(), func(order gatewayOrderResponse) string {
			return order.ID
		}),
		response.Required("count", response.Int64(), func(order gatewayOrderResponse) int64 {
			return order.Count
		}),
	)
}
