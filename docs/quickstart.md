---
title: Run your first endpoint
sidebarTitle: Quickstart
description: Install HTTPAPI, serve a typed JSON endpoint, and call it locally.
---

You will build one `POST /tasks` endpoint. Invalid JSON stops at the contract boundary; accepted input reaches a typed handler.

## Prerequisites

- Go 1.27 or later
- A Go module you can run locally

## Install HTTPAPI

```bash
go get github.com/inttegro/httpapi
```

## Define and serve the endpoint

Create `main.go`:

```go
package main

import (
	"log"
	"net/http"

	"github.com/inttegro/httpapi/endpoint"
	"github.com/inttegro/httpapi/param"
	"github.com/inttegro/httpapi/response"
	"github.com/inttegro/httpapi/server"
)

type createTaskRequest struct {
	Name string
}

var createTaskBody = param.JSON[createTaskRequest]().
	Param(param.Required("name", param.String()).
		Null(param.NullRejected).
		MinSize(1).
		MaxSize(160).
		Parse(param.NonEmptyTrimmedString)).
	Parse(func(values param.Values) (createTaskRequest, error) {
		return createTaskRequest{
			Name: param.Must[string](values, "name"),
		}, nil
	})

var createTask = endpoint.DefineJSONEndpoint(endpoint.JSONEndpointSpec[createTaskRequest]{
	Method:  endpoint.POST,
	Path:    "/tasks",
	Request: createTaskBody,
	Respond: func(_ *endpoint.Req, input createTaskRequest) *response.Res {
		return response.JSON(http.StatusCreated, map[string]string{
			"id":   "task_123",
			"name": input.Name,
		})
	},
	Operation: endpoint.OperationSpec{
		ID:      "createTask",
		Summary: "Create a task",
		Kind:    endpoint.OperationKindWrite,
	},
})

func main() {
	mux := endpoint.NewMux()
	mux.MustMountEndpoints(createTask)

	srv := server.New(server.Config{
		Port: server.PortFromEnv("PORT", server.DefaultPort),
		Mux:  mux,
	})

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
```

## Run and call it

Start the service:

```bash
go run .
```

Send a request from another terminal:

```bash
curl -i http://localhost:8080/tasks \
  -H 'Content-Type: application/json' \
  --data '{"name":"  Ship the docs  "}'
```

The parser removes the surrounding spaces before the handler receives the value:

```json
{"id":"task_123","name":"Ship the docs"}
```

Try an empty name. The handler will not run; HTTPAPI returns a structured parameter error instead.

## What to add next

1. Replace the example map with a declared [response shape](/response-projection).
2. Add authorization, timeouts, limits, and operation metadata in the [endpoint contract](/endpoint-runtime).
3. Install your authentication and persistence adapters through [server wiring](/server-wiring).
