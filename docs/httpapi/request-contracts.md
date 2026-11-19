---
title: Parse requests into usable values
sidebarTitle: Requests
description: Accept JSON, normalize it, and give the handler a complete typed value without a second validation pass.
---

Define a `param.Request` beside the endpoint that consumes it. The parser should either return a value your domain service can use immediately or return one structured parameter error.

## Define the accepted JSON

```go
type createTaskParams struct {
	Name     string
	Priority string
}

var createTaskRequest = param.JSON[createTaskParams]().
	Param(param.Required("name", param.String()).
		Null(param.NullRejected).
		MinSize(1).
		MaxSize(160).
		Parse(param.NonEmptyTrimmedString)).
	Param(param.Optional("priority", param.Enum("low", "standard", "high")).
		Null(param.NullRejected)).
	Parse(func(values param.Values) (createTaskParams, error) {
		priority := "standard"
		if value, ok := param.Get[string](values, "priority"); ok {
			priority = value
		}

		return createTaskParams{
			Name:     param.Must[string](values, "name"),
			Priority: priority,
		}, nil
	})
```

This contract trims `name`, rejects an empty or oversized value, rejects `null`, constrains `priority` to a declared enum, and supplies an application default. The handler does not repeat those checks.

## Bind parsing to the endpoint

```go
var createTask = endpoint.DefineJSONEndpoint(
	endpoint.JSONEndpointSpec[createTaskParams]{
		Method:  endpoint.POST,
		Path:    "/tasks",
		Request: createTaskRequest,
		Respond: func(r *endpoint.Req, input createTaskParams) *response.Res {
			return createTaskInDomain(r.Context(), input)
		},
	},
)
```

`DefineJSONEndpoint` uses the same parser for runtime acceptance and request-body transcription. It calls the responder only after parsing succeeds and renders parser failures through HTTPAPI's standard error envelope.

## Make presence and null deliberate

Absent and `null` are different inputs. Declare both decisions:

- `Required` or `Optional` controls presence.
- `NullRejected`, `NullAccepted`, or another explicit null policy controls JSON `null`.
- `OneOf` requires exactly one member of a group.
- `MutuallyExclusive` allows zero or one member.
- `AtLeastOneOf` requires one or more members.

Use `DiscriminatedObject` when one string field selects the rest of an object's shape. Use `ArrayOf` when each array item needs its own nested parser.

## Keep live-state rules in the domain

The request contract can prove that `customer_id` has the right wire format. It cannot prove that the customer still exists or belongs to the active application. Perform those checks in the domain service, inside the transaction that owns the state.

<Note>
  A parameter restricted with `AvailableTo` is reported as unexpected when another caller sends it. That preserves the hidden contract instead of revealing the parameter's name or allowed callers.
</Note>

Next, define [caller authority](/httpapi/callers-and-authority) and [response shapes](/httpapi/response-projection) with the same caller labels.
