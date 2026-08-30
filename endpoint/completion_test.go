package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zebodotdev/httpapi/cost"
	"github.com/zebodotdev/httpapi/param"
	"github.com/zebodotdev/httpapi/response"
)

func TestEndpointCompletionSinkReceivesSuccessfulRequest(t *testing.T) {
	events := captureEndpointCompletions(t)
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/tasks/create",
		Handler: func(r *Req) {
			response.RenderJSON(r, http.StatusCreated, map[string]bool{"ok": true})
		},
		Operation: OperationSpec{
			ID:      "createTask",
			Summary: "Create task",
			Kind:    OperationKindWrite,
		},
	})
	req := httptest.NewRequest(POST, "/tasks/create", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusCreated, res.Body.String())
	}
	event := onlyEndpointCompletion(t, events)
	if event.Outcome != CompletionOutcomeHandled {
		t.Fatalf("outcome = %q, want %q", event.Outcome, CompletionOutcomeHandled)
	}
	if event.Status != http.StatusCreated {
		t.Fatalf("completion status = %d, want %d", event.Status, http.StatusCreated)
	}
	if event.Endpoint.Method != POST || event.Endpoint.Pattern != "/tasks/create" {
		t.Fatalf("endpoint = %s %s", event.Endpoint.Method, event.Endpoint.Pattern)
	}
	if event.Endpoint.Operation.ID != "createTask" {
		t.Fatalf("operation id = %q, want createTask", event.Endpoint.Operation.ID)
	}
	if event.Endpoint.OperationKind() != OperationKindWrite {
		t.Fatalf("operation kind = %q, want %q", event.Endpoint.OperationKind(), OperationKindWrite)
	}
	if event.Request == nil || event.Request.ID == "" {
		t.Fatalf("request = %#v, want request id", event.Request)
	}
	if event.Duration <= 0 {
		t.Fatalf("duration = %v, want positive duration", event.Duration)
	}
	if event.ResponseSizeBytes == 0 {
		t.Fatal("response size was not recorded")
	}
}

func TestEndpointCompletionSinkReceivesCostEvent(t *testing.T) {
	events := captureEndpointCompletions(t)
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/orders/create",
		Handler: func(r *Req) {
			err := cost.Record(r.Context(), cost.NewUsageUnit(
				"aws",
				"dynamodb",
				"put-item",
				"write-request-unit",
				cost.Whole(1),
			).WithLabel("table", "orders"))
			if err != nil {
				t.Errorf("cost.Record returned error: %v", err)
			}
			response.RenderNoContent(r)
		},
		Operation: OperationSpec{
			ID:      "createOrder",
			Summary: "Create order",
			Kind:    OperationKindWrite,
		},
		Priority: PriorityHigh,
	})
	req := httptest.NewRequest(POST, "/orders/create", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	req.Header.Set("x-request-id", "req_external")
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusNoContent, res.Body.String())
	}
	event := onlyEndpointCompletion(t, events)
	if event.Cost.Empty() {
		t.Fatal("completion cost event did not include usage")
	}
	if event.Cost.Request.ID != event.Request.ID {
		t.Fatalf("cost request id = %q, want %q", event.Cost.Request.ID, event.Request.ID)
	}
	if event.Cost.Operation.ID != event.Request.ID ||
		event.Cost.Operation.RootID != event.Request.ID {
		t.Fatalf("cost operation metadata = %#v, want request-rooted operation", event.Cost.Operation)
	}
	if event.Cost.Operation.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("cost trace id = %q, want traceparent trace id", event.Cost.Operation.TraceID)
	}
	if event.Cost.Operation.CausationRequestID != "req_external" {
		t.Fatalf("cost causation request id = %q, want req_external", event.Cost.Operation.CausationRequestID)
	}
	if event.Cost.Operation.Name != "createOrder" {
		t.Fatalf("cost operation name = %q, want createOrder", event.Cost.Operation.Name)
	}
	if event.Endpoint.Operation.Accounting.Cost != CostAccountingDefault {
		t.Fatalf("completion operation = %#v, want default accounting", event.Endpoint.Operation)
	}
	if !event.Endpoint.CostAccountingEnabled() {
		t.Fatal("completion endpoint cost accounting should be enabled by default")
	}
	if event.Cost.Request.Status != http.StatusNoContent {
		t.Fatalf("cost status = %d, want %d", event.Cost.Request.Status, http.StatusNoContent)
	}
	if event.Cost.Request.Outcome != string(CompletionOutcomeHandled) {
		t.Fatalf("cost outcome = %q, want handled", event.Cost.Request.Outcome)
	}
	if event.Cost.Endpoint.OperationID != "createOrder" ||
		event.Cost.Endpoint.OperationKind != string(OperationKindWrite) ||
		event.Cost.Endpoint.Priority != string(PriorityHigh) {
		t.Fatalf("cost endpoint metadata = %#v", event.Cost.Endpoint)
	}
	if len(event.Cost.Usage) != 1 {
		t.Fatalf("cost usage units = %d, want 1", len(event.Cost.Usage))
	}
	usage := event.Cost.Usage[0]
	if usage.Provider != "aws" || usage.Service != "dynamodb" ||
		usage.SKU != "put-item" || usage.Unit != "write-request-unit" {
		t.Fatalf("cost usage = %#v", usage)
	}
	if usage.Quantity != cost.Whole(1) {
		t.Fatalf("usage quantity = %s, want 1", usage.Quantity)
	}
	if usage.Labels["table"] != "orders" {
		t.Fatalf("usage label = %q, want orders", usage.Labels["table"])
	}
}

func TestEndpointCompletionSinkUsesFallbackCostOperationName(t *testing.T) {
	events := captureEndpointCompletions(t)
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/orders/create",
		Handler: func(r *Req) {
			if err := r.AddCostUnit(
				"aws",
				"dynamodb",
				"put-item",
				"write-request-unit",
				cost.Whole(1),
			); err != nil {
				t.Errorf("AddCostUnit returned error: %v", err)
			}
			response.RenderNoContent(r)
		},
		Operation: OperationSpec{
			Accounting: AccountingSpec{
				Cost: CostAccountingEnabled,
			},
		},
	})
	req := httptest.NewRequest(POST, "/orders/create", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusNoContent, res.Body.String())
	}
	event := onlyEndpointCompletion(t, events)
	if event.Endpoint.Operation.Accounting.Cost != CostAccountingEnabled {
		t.Fatalf("completion operation = %#v, want enabled accounting", event.Endpoint.Operation)
	}
	if event.Cost.Operation.Name != "POST /orders/create" {
		t.Fatalf("cost operation name = %q, want POST /orders/create", event.Cost.Operation.Name)
	}
}

func TestEndpointCompletionSinkSuppressesCostEventWhenAccountingDisabled(t *testing.T) {
	events := captureEndpointCompletions(t)
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/orders/create",
		Handler: func(r *Req) {
			if err := r.AddCostUnit(
				"aws",
				"dynamodb",
				"put-item",
				"write-request-unit",
				cost.Whole(1),
			); err != nil {
				t.Errorf("AddCostUnit returned error: %v", err)
			}
			response.RenderNoContent(r)
		},
		Operation: OperationSpec{
			ID: "createOrder",
			Accounting: AccountingSpec{
				Cost: CostAccountingDisabled,
			},
		},
	})
	req := httptest.NewRequest(POST, "/orders/create", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusNoContent, res.Body.String())
	}
	event := onlyEndpointCompletion(t, events)
	if event.Endpoint.Operation.Accounting.Cost != CostAccountingDisabled {
		t.Fatalf("completion operation = %#v, want disabled accounting", event.Endpoint.Operation)
	}
	if !event.Cost.Empty() {
		t.Fatalf("disabled endpoint cost event = %#v, want empty", event.Cost)
	}
	if event.Cost.Operation.Name != "" {
		t.Fatalf("disabled endpoint cost operation name = %q, want empty", event.Cost.Operation.Name)
	}
}

func TestEndpointCompletionSinkReceivesTypedParseFailure(t *testing.T) {
	events := captureEndpointCompletions(t)
	called := false
	endpoint := DefineJSONEndpoint(JSONEndpointSpec[typedJSONParams]{
		Method:  POST,
		Path:    "/typed",
		Request: typedJSONRequest(),
		Handler: func(r *Req, params typedJSONParams) {
			called = true
			response.RenderJSON(r, http.StatusOK, params)
		},
	})
	req := httptest.NewRequest(POST, "/typed", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if called {
		t.Fatal("typed handler was called")
	}
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusBadRequest, res.Body.String())
	}
	event := onlyEndpointCompletion(t, events)
	if event.Outcome != CompletionOutcomeParseFailed {
		t.Fatalf("outcome = %q, want %q", event.Outcome, CompletionOutcomeParseFailed)
	}
	if event.Error == nil || event.Error.Code != string(param.CodeMissing) {
		t.Fatalf("completion error = %#v, want missing parameter error", event.Error)
	}
}

func TestEndpointCompletionSinkReceivesAccessFailure(t *testing.T) {
	events := captureEndpointCompletions(t)
	called := false
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/private",
		Access: EndpointAccessSpec{
			Authorization: RequiredAuthorization(AuthorizationKindBearer),
		},
		Handler: func(r *Req) {
			called = true
			response.RenderNoContent(r)
		},
	})
	req := httptest.NewRequest(POST, "/private", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	endpoint.Handler()(res, req)

	if called {
		t.Fatal("handler was called")
	}
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusUnauthorized, res.Body.String())
	}
	event := onlyEndpointCompletion(t, events)
	if event.Outcome != CompletionOutcomeAccessDenied {
		t.Fatalf("outcome = %q, want %q", event.Outcome, CompletionOutcomeAccessDenied)
	}
	if event.Request == nil || event.Request.AuthorizationFailure == nil {
		t.Fatalf("authorization failure = %#v", event.Request)
	}
	if event.Error == nil || event.Error.Code != endpointAuthenticationRequiredCode {
		t.Fatalf("completion error = %#v, want endpoint auth error", event.Error)
	}
}

func TestEndpointCompletionSinkReceivesPanicBeforePropagation(t *testing.T) {
	events := captureEndpointCompletions(t)
	endpoint := DefineEndpoint(EndpointSpec{
		Method: POST,
		Path:   "/panic",
		Handler: func(r *Req) {
			panic("boom")
		},
	})
	req := httptest.NewRequest(POST, "/panic", strings.NewReader(`{}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	func() {
		defer func() {
			got := recover()
			if got != "boom" {
				t.Fatalf("panic = %#v, want boom", got)
			}
		}()
		endpoint.Handler()(res, req)
	}()

	event := onlyEndpointCompletion(t, events)
	if event.Outcome != CompletionOutcomePanicked {
		t.Fatalf("outcome = %q, want %q", event.Outcome, CompletionOutcomePanicked)
	}
	if event.Status != http.StatusInternalServerError {
		t.Fatalf("completion status = %d, want %d", event.Status, http.StatusInternalServerError)
	}
	if event.Panic == nil || event.Panic.Type != "string" || event.Panic.Value != "boom" {
		t.Fatalf("panic metadata = %#v", event.Panic)
	}
	if event.Error == nil {
		t.Fatal("completion error was not recorded for panic response")
	}
}

func captureEndpointCompletions(t *testing.T) *[]Completion {
	t.Helper()
	events := []Completion{}
	restore := ConfigureCompletionSink(CompletionSinkFunc(
		func(ctx context.Context, completion Completion) error {
			events = append(events, completion)
			return nil
		},
	))
	t.Cleanup(restore)
	return &events
}

func onlyEndpointCompletion(t *testing.T, events *[]Completion) Completion {
	t.Helper()
	if events == nil {
		t.Fatal("events pointer is nil")
	}
	if len(*events) != 1 {
		t.Fatalf("completion events = %d, want 1", len(*events))
	}
	return (*events)[0]
}
