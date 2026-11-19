---
title: Shape responses at the boundary
sidebarTitle: Responses
description: Return domain values while HTTPAPI selects the public fields available to the active caller.
---

Define response shapes once, then use them both when rendering and when describing the endpoint. A shape prevents storage fields from becoming API fields by accident.

## Define the public object

```go
type taskView struct {
	ID           string
	Status       string
	InternalNote string
}

var taskShape = response.Object[taskView](
	response.Required("id", response.String(),
		func(task taskView) string { return task.ID }),
	response.Required("status", response.String(),
		func(task taskView) string { return task.Status }),
	response.Optional("internal_note", response.String(),
		func(task taskView) (string, bool) {
			return task.InternalNote, task.InternalNote != ""
		}).AvailableTo(Worker),
)

var taskEnvelope = response.Envelope(
	response.RequiredField("task", taskShape),
)
```

The getter for `internal_note` is not used when the active caller cannot see that attribute.

## Render and describe the same shape

```go
var lookupTask = endpoint.DefineEndpoint(endpoint.EndpointSpec{
	Method: endpoint.POST,
	Path:   "/tasks/lookup",
	Respond: func(_ *endpoint.Req) *response.Res {
		task := taskView{ID: "task_123", Status: "open"}
		return response.JSON(http.StatusOK, taskEnvelope.Body(
			response.Field("task", task),
		))
	},
	Responses: []endpoint.ResponseContract{
		endpoint.ResponseBody(http.StatusOK, "Task found.", taskEnvelope),
	},
})
```

`taskEnvelope.Body` delays projection until HTTPAPI knows the response caller and any property selection attached to the request. `ResponseBody` gives the runtime and transcribers the same shape metadata.

## Let clients select fields safely

Compatible JSON endpoints accept standard response projection metadata:

```json
{
	"request_meta": {
		"response": {
			"projection": {
				"task": {
					"pick": ["id", "status"]
				}
			}
		}
	}
}
```

Each top-level envelope field is a projection resource. Within one resource, a client may use `pick` or `omit`, not both. The endpoint validates the resource names, property paths, and caller visibility against its successful JSON response contracts before invoking application code.

Use `response.Project` when a domain object first needs a small response-facing view. Use `response.Envelope` for stable top-level resources and `ExactlyOneOf` for mutually exclusive response alternatives.

<Note>
  Response projection controls emitted fields. It does not authorize access to the endpoint or the underlying resource.
</Note>
