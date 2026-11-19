---
title: Define the endpoint runtime contract
sidebarTitle: Endpoint contract
description: Put access, limits, idempotency, timeouts, operation identity, and response contracts beside the handler.
---

Use `EndpointSpec` for general endpoints and `JSONEndpointSpec` for typed JSON endpoints. The fields on these specs are the policy the runtime executes—not documentation-only annotations.

## Declare the complete endpoint

```go
var createTask = endpoint.DefineJSONEndpoint(
	endpoint.JSONEndpointSpec[createTaskParams]{
		Method:  endpoint.POST,
		Path:    "/tasks",
		Request: createTaskRequest,
		Respond: createTaskResponder,
		Access: endpoint.EndpointAccessSpec{
			Authorization: endpoint.RequiredAuthorization(
				endpoint.AuthorizationKindBearer,
			),
			Callers: []caller.Caller{PublicAPI, Dashboard},
		},
		Idempotency: endpoint.EndpointIdempotencySpec{
			Enabled: true,
		},
		Priority: endpoint.EndpointPriorityHigh,
		Timeout: endpoint.EndpointTimeoutSpec{
			ReadBody: 2 * time.Second,
			Handler:  10 * time.Second,
			Write:    2 * time.Second,
		},
		Limits: endpoint.EndpointLimitsSpec{
			MaxRequestBytes: 64 << 10,
		},
		Operation: endpoint.OperationSpec{
			ID:      "createTask",
			Summary: "Create a task",
			Kind:    endpoint.OperationKindWrite,
		},
		Responses: []endpoint.ResponseContract{
			endpoint.ResponseBody(
				http.StatusCreated,
				"Task created.",
				taskEnvelope,
			),
		},
	},
)
```

Prefer `Respond`, which returns a `*response.Res`. `Handler` remains available for integrations that render directly onto the request.

## Expect the runtime to finish some requests

The handler runs only after the relevant boundary checks pass. The runtime can complete a request because of:

- the wrong method or media type;
- an oversized or unreadable body;
- failed authentication, caller access, or endpoint controls;
- invalid request metadata or response projection;
- an idempotency conflict or unavailable store; or
- an expired handler timeout.

Completion and audit sinks still observe the final result. A handler panic is recorded before HTTPAPI re-panics to preserve `net/http` behavior.

## Give every operation a stable identity

`Operation.ID` connects the endpoint to generated documents, endpoint catalogs, completion events, availability controls, and cost accounting. Keep it stable across route changes.

Declare `Operation.Kind` explicitly as `read` or `write`. HTTPAPI does not infer mutation from the HTTP method, and groups do not supply a default kind.

## Share defaults through groups

```go
tasks := endpoint.EndpointGroup{
	PathPrefix: "/v1",
	Auth: endpoint.RequiredAuthorization(
		endpoint.AuthorizationKindBearer,
	),
	Callers:  []caller.Caller{PublicAPI, Dashboard},
	Priority: endpoint.EndpointPriorityStandard,
	Timeout: endpoint.EndpointTimeoutSpec{
		Handler: 5 * time.Second,
	},
	Endpoints: []endpoint.Endpoint{createTask, lookupTask},
}
```

Groups supply shared access, priority, limit, timeout, operation-summary, accounting, and route defaults. Endpoint values override them where allowed. Caller restrictions intersect, so a group cannot widen a restricted endpoint. Operation IDs and kinds remain endpoint-specific.

Use aliases when the same operation needs another route. An alias shares the handler and every endpoint contract; it does not create a second operation.
