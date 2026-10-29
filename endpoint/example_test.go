package endpoint_test

import (
	"context"
	"net/http"
	"time"

	"github.com/inttegro/httpapi/endpoint"
	"github.com/inttegro/httpapi/param"
	"github.com/inttegro/httpapi/response"
)

type createTaskRequest struct {
	Name string
}

func ExampleDefineEndpoint() {
	createTask := endpoint.DefineEndpoint(endpoint.EndpointSpec{
		Method: endpoint.POST,
		Path:   "/tasks/create",
		Handler: func(r *endpoint.Req) {
			response.RenderJSON(r, http.StatusCreated, map[string]string{
				"id": "task_123",
			})
		},
		Operation: endpoint.OperationSpec{
			ID:      "createTask",
			Summary: "Create a task",
		},
		Priority: endpoint.EndpointPriorityHigh,
		Timeout: endpoint.EndpointTimeoutSpec{
			Handler: 5 * time.Second,
			Write:   2 * time.Second,
		},
		Limits: endpoint.EndpointLimitsSpec{
			MaxRequestBytes: 1 << 20,
		},
	})

	_ = createTask
}

func ExampleDefineJSONEndpoint() {
	createTaskParser := param.JSON[createTaskRequest]().
		Param(param.Required("name", param.String()).
			Parse(param.NonEmptyTrimmedString)).
		Parse(func(values param.Values) (createTaskRequest, error) {
			return createTaskRequest{
				Name: param.Must[string](values, "name"),
			}, nil
		})

	createTask := endpoint.DefineJSONEndpoint(endpoint.JSONEndpointSpec[createTaskRequest]{
		Method:  endpoint.POST,
		Path:    "/tasks/create",
		Request: createTaskParser,
		Handler: func(r *endpoint.Req, input createTaskRequest) {
			response.RenderJSON(r, http.StatusCreated, map[string]string{
				"name": input.Name,
			})
		},
		Operation: endpoint.OperationSpec{
			ID:      "createTask",
			Summary: "Create a task",
		},
	})

	_ = createTask
}

func ExampleNewMux() {
	createTask := endpoint.DefineEndpoint(endpoint.EndpointSpec{
		Method: endpoint.POST,
		Path:   "/create",
		Handler: func(r *endpoint.Req) {
			response.RenderNoContent(r)
		},
		Operation: endpoint.OperationSpec{
			ID:      "createTask",
			Summary: "Create a task",
		},
	})

	lookupTask := endpoint.DefineEndpoint(endpoint.EndpointSpec{
		Method: endpoint.GET,
		Path:   "/lookup",
		Handler: func(r *endpoint.Req) {
			response.RenderJSON(r, http.StatusOK, map[string]string{
				"id": "task_123",
			})
		},
		Operation: endpoint.OperationSpec{
			ID:      "lookupTask",
			Summary: "Look up a task",
		},
	})

	group := endpoint.EndpointGroup{
		PathPrefix: "/tasks",
		Endpoints:  []endpoint.Endpoint{createTask, lookupTask},
		Route: endpoint.RouteSpec{
			Backend: endpoint.RouteBackend{
				Address:  "https://tasks.internal",
				PathMode: endpoint.RoutePathModeAppend,
				Timeout:  10 * time.Second,
			},
		},
		Priority: endpoint.EndpointPriorityStandard,
		Timeout: endpoint.EndpointTimeoutSpec{
			Handler: 5 * time.Second,
		},
	}

	mux := endpoint.NewMux()
	mux.MustMount(group)

	_ = mux
}

func ExampleConfigureCompletionSink() {
	restore := endpoint.ConfigureCompletionSink(endpoint.CompletionSinkFunc(
		func(ctx context.Context, completion endpoint.Completion) error {
			_ = ctx
			_ = completion.Endpoint.Operation.ID
			_ = completion.Status
			_ = completion.Duration
			_ = completion.Outcome
			return nil
		},
	))
	defer restore()
}
