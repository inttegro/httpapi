# HTTP API

`github.com/zebodotdev/httpapi` is a reusable HTTP contract layer for Go
services.

It helps service teams define endpoint contracts, parse request bodies into
domain-ready values, render caller-aware responses, enforce runtime endpoint
requirements, and transcribe endpoint groups into OpenAPI documents.

The module is intentionally infrastructure-neutral. It does not know about your
database, auth provider, logging stack, deployment platform, or service
configuration system. Applications plug those pieces in at the package
boundaries.

## Install

```sh
go get github.com/zebodotdev/httpapi
```

## Mental Model

Use the narrow package that owns the behavior you need:

- `caller` defines stable labels for trusted request sources.
- `request` turns an incoming `*http.Request` into a safe `Req` with buffered
  body, request ID, caller, session, and redacted audit output.
- `cost` records provider-neutral usage units that completion sinks can price or
  export outside httpapi.
- `param` parses JSON request bodies into endpoint-owned domain parameters.
- `response` builds responses, projects response shapes, filters
  caller-specific attributes, and writes HTTP responses.
- `endpoint` defines endpoint contracts, provides the default mux, and enforces
  method, content type, auth, caller availability, idempotency, timeout, and
  request-size policy.
- `server` builds an `http.Server` around an `endpoint.Mux` or custom
  `http.Handler` with conservative defaults, CORS middleware, and an explicit
  middleware chain.
- `openapi/*` transcribes endpoint metadata into target documents.

The important separation is:

- request parsing prepares an acceptable request;
- endpoint definitions describe what a route requires;
- param parsing accepts and normalizes endpoint payload data;
- response shapes describe what an endpoint returns.

Do not put endpoint requirements on `request.Req`. `Req` should not know which
endpoint will handle it.

## Callers

A `caller.Caller` is an application-defined label for the trusted source of a
request. It is not an auth scheme. The same caller values can restrict endpoint
access, request parameters, and response attributes.

Define callers once and reuse the values:

```go
package tasksapi

import "github.com/zebodotdev/httpapi/caller"

var (
	PublicAPI = caller.Define("public-api")
	Worker    = caller.Define("worker")
	Dashboard = caller.Define("dashboard")
)
```

Trusted middleware should attach the active caller with
`request.ContextWithCaller` before the endpoint runtime builds `request.Req`.

When no caller restriction is configured, the object is available to every
caller. This applies consistently to endpoint groups, endpoints, params, and
response attributes.

## Request Params

Use `param` to parse, not to perform a separate validation pass. Parsing should
return a complete value that downstream code can safely use, or a `*param.Error`
that is safe to convert into an API error response.

Define parsers once near the endpoint:

```go
type createTaskParams struct {
	Title      string
	Tags       []string
	AssigneeID string
	Automation automationParams
}

type automationParams struct {
	Name  string
	RunAt *time.Time
}

var createTaskRequest = param.JSON[createTaskParams]().
	Param(param.Required("title", param.String()).
		Null(param.NullRejected).
		MinSize(1).
		MaxSize(160).
		Parse(param.NonEmptyTrimmedString)).
	Param(param.Optional("tag_names", param.Array[string]()).
		MaxItems(20).
		Parse(param.TrimmedStringList)).
	Param(param.MutuallyExclusive(
		param.Optional("assignee_id", param.String()).
			Parse(parseTaskAssigneeID),
		param.Optional("automation",
			param.Object[automationParams]().
				Param(param.Required("name", param.String()).
					Parse(param.NonEmptyTrimmedString)).
				Param(param.Optional("run_at", param.String()).
					Parse(param.OptionalRFC3339TimestampPointer)).
				Parse(parseAutomation),
		).AvailableTo(Worker),
	)).
	Parse(parseCreateTask)
```

Use grouped declarations such as `param.MutuallyExclusive(...)` when a set of
optional parameters owns a presence rule. The group is passed to `Param`, so the
accepted parameters and their relationship stay together in the request shape.
Call `.Required()` on a mutually-exclusive group when exactly one of the grouped
parameters must be present.

Use `param.Enum(...)` for string parameters with fixed allowed values. Enum
membership is checked before custom parsers run, and the allowed values are
included in request metadata for transcription:

```go
Param(param.Required("status", param.Enum("draft", "active", "archived")))
```

Use `param.DiscriminatedObject(...)` when a string parameter selects the object
shape to parse. Variant object shapes omit the discriminator parameter because
the helper consumes it before parsing the selected branch:

```go
var lineItemShape = param.DiscriminatedObject[lineItemParams]("type").
	Variant("product",
		param.Object[lineItemParams]().
			Param(param.Required("product_id", param.String())).
			Parse(parseProductLineItem),
	).
	Variant("fee",
		param.Object[lineItemParams]().
			Param(param.Required("amount", param.Int())).
			Parse(parseFeeLineItem),
	)
```

Inside the final parser, use `param.Must` for required parameters and
`param.Get` for optional parameters:

```go
func parseCreateTask(values param.Values) (createTaskParams, error) {
	params := createTaskParams{
		Title: param.Must[string](values, "title"),
	}

	if tags, ok := param.Get[[]string](values, "tag_names"); ok {
		params.Tags = tags
	}
	if assigneeID, ok := param.Get[string](values, "assignee_id"); ok {
		params.AssigneeID = assigneeID
	}
	if automation, ok := param.Get[automationParams](values, "automation"); ok {
		params.Automation = automation
	}

	return params, nil
}
```

Runtime usage stays direct:

```go
params, err := createTaskRequest.Parse(
	r.Body,
	param.WithRequestCaller(r),
)
if err != nil {
	renderParamError(r, err)
	return
}
```

Services choose their own public error envelope. A small adapter is usually
enough:

```go
func renderParamError(r response.Target, err *param.Error) {
	if err == nil {
		response.RenderErr(r, nil)
		return
	}

	response.RenderErr(r, erreur.InvalidParam(
		string(err.Code),
		err.Message,
		"",
	))
}
```

Restricted params deliberately fail as unexpected parameters when sent by a
caller that cannot use them. That avoids revealing that a hidden parameter
exists or which caller is allowed to send it.

## Cost Usage

Use `cost` to observe metered usage for a request, job, workflow, activity, or
provider call without putting pricing policy in httpapi. A usage unit names the
provider, service, SKU, unit, and exact decimal quantity consumed by an
operation. It does not contain USD prices, billing accounts, invoices,
discounts, margins, or reconciliation state.

For HTTP requests, httpapi creates a request operation recorder and attaches it
to `r.Context()` before authentication and handler code run. Handlers can add
usage directly to the request:

```go
err := r.AddCostUsage(cost.NewUsageUnit(
	"aws",
	"dynamodb",
	"put-item",
	"write-request-unit",
	cost.Whole(1),
).WithLabel("table", "orders"))
if err != nil {
	response.RenderErr(r, erreur.Unexpected())
	return
}
```

For fractional units, use an exact decimal quantity instead of `float64`:

```go
quantity := cost.MustDecimal(125, 2) // 1.25
_ = r.AddCostUnit("gcp", "cloud-run", "cpu", "vcpu-second", quantity)
```

Lower-level code that only receives context can use the context recorder:

```go
func loadTask(ctx context.Context, id string) (*Task, error) {
	if err := cost.RecordUnit(
		ctx,
		"aws",
		"dynamodb",
		"get-item",
		"read-request-unit",
		cost.Whole(1),
	); err != nil {
		return nil, err
	}

	return repo.LoadTask(ctx, id)
}
```

Endpoint completion observers receive the final operation event:

```go
restore := endpoint.ConfigureCompletionSink(endpoint.CompletionSinkFunc(
	func(ctx context.Context, completion endpoint.Completion) error {
		event := completion.Cost
		if event.Empty() {
			return nil
		}

		return exportCostUsage(ctx, event)
	},
))
defer restore()
```

Service-owned sinks decide how to aggregate, price, persist, or export the
event. The operation metadata contains `operation_id`, `root_operation_id`,
`parent_operation_id`, `trace_id`, and `causation_request_id` where available,
so async child work can be correlated with the original request.

Request cost recording is active by default so existing completion sinks keep
receiving usage when handlers record it. To mark an endpoint as intentionally
cost-accounted, set `Operation.Accounting` on the endpoint, or on an
`EndpointGroup` as a default. Set `CostAccountingDisabled` to suppress
completion cost events for an endpoint. The accounting operation identity is
`Operation.ID`; if it is empty, endpoint completion falls back to
`METHOD pattern`.

Background jobs and workflows can start child recorders from an existing
context, record usage anywhere in their call stack, and flush a final event to a
durable service-owned sink:

```go
type operationSink struct{}

func (operationSink) RecordOperation(ctx context.Context, event cost.OperationEvent) error {
	// Price event.Usage with service policy, persist an estimate keyed by
	// event.Operation.ID/RootID, and reconcile later against provider invoices.
	return nil
}

func runWorkflow(ctx context.Context, workflowID string, sink cost.OperationSink) error {
	ctx, recorder := cost.StartChild(ctx, cost.Operation{
		ID:   workflowID,
		Name: "settle_payouts",
		Kind: "workflow",
	})
	defer recorder.Flush(ctx, sink)

	return executeWorkflow(ctx)
}
```

When service code wants to separate estimation from persistence, implement
`cost.Estimator` and `cost.EstimateSink`. `cost.EstimateEvent` allows a
downstream estimate to carry `currency` and exact decimal `amount`, but httpapi
still does not define how that amount is calculated.

`httpapi` does not implement DynamoDB, GCP, AWS price lookup, cloud invoice
matching, or application pricing policy. Keep labels low-cardinality and do not
attach request bodies, authorization headers, raw tokens, API keys, or other
secret material.

## Responses

Handlers can render ordinary payloads:

```go
response.RenderJSON(r, http.StatusOK, map[string]any{
	"status": "ready",
})

response.RenderNoContent(r)
response.RenderRedirect(r, http.StatusSeeOther, "https://example.test/next")
response.RenderStream(r, http.StatusOK, "application/pdf", header, reader)
```

Use response shapes when the JSON object is part of the endpoint contract or
when attributes have caller availability rules. A response shape is explicit:
each emitted JSON key is named with its JSON type and extractor.

For stored/domain values that need nil guards, defaulting, or redaction before
attribute extraction, define a small response view and adapt the domain value
with `response.Project`:

```go
type taskView struct {
	task Task
}

func taskValue(task *Task) taskView {
	if task == nil {
		return taskView{}
	}
	return taskView{task: *task}
}

func (task taskView) ID() string { return task.task.ID }
func (task taskView) Status() string { return task.task.Status }
func (task taskView) CreatedAt() time.Time { return task.task.CreatedAt }
func (task taskView) InternalNote() (string, bool) {
	return task.task.InternalNote, task.task.InternalNote != ""
}

var taskResponse = response.Project(
	response.Object[taskView](
		response.Required("id", response.String(), taskView.ID),
		response.Required("status", response.String(), taskView.Status),
		response.Required("created_at", response.Time(), taskView.CreatedAt),
		response.Optional("internal_note", response.String(), taskView.InternalNote).
			AvailableTo(Worker, Dashboard),
	),
	taskValue,
)
```

Render the shaped body from the handler:

```go
response.RenderJSON(r, http.StatusCreated, taskResponse.Body(task))
```

When the target is `*request.Req` or `*endpoint.Req`, `response` derives the
caller from `r.RequestCaller()`. Do not pass the caller manually from handlers.

Use `response.Object[T]` directly when the handler already has a response view
value. Use `response.Project(shape, prepare)` when the handler has a domain
value and the shape should read against a prepared response view.

Use `response.Envelope(...)` when the endpoint only needs to wrap one or more
already-shaped values under stable top-level keys. This removes endpoint-local
carrier structs such as `type Response struct { Task *taskView; Err *erreur.Error }`:

```go
var taskEnvelope = response.Envelope(
	response.OptionalField("task", taskResponse),
	response.OptionalField("error", response.ErrorObjectShape),
)

func created(r response.Target, task *Task) {
	response.RenderJSON(r, http.StatusCreated, taskEnvelope.Body(
		response.Field("task", task),
	))
}

func failed(r response.Target, err *erreur.Error) {
	response.RenderJSON(r, err.Status, taskEnvelope.Body(
		response.Field("error", err),
	))
}
```

An envelope must define at least one accepted field, and each rendered response
must emit at least one real field for the active caller. Each accepted field
must have a distinct Go value type. If two JSON fields would otherwise share an
underlying type, define small named types and use `response.Project` to map each
named type back to the JSON scalar shape. `OptionalField` values that are nil,
including typed nil pointers, maps, and slices, are omitted and do not satisfy
the envelope's non-empty requirement. `RequiredField` describes schema and
per-field requiredness: a missing or nil required value panics, but callers do
not need a required field just to make the envelope non-empty. Unexpected fields
still panic.

`Envelope` also implements `response.Shape`, so it can describe a nested object
without defining a one-off Go struct:

```go
type pageNumber int
type pageSize int

var pageNumberShape = response.Project(
	response.Int(),
	func(value pageNumber) int { return int(value) },
)

var pageSizeShape = response.Project(
	response.Int(),
	func(value pageSize) int { return int(value) },
)

var pageShape = response.Envelope(
	response.RequiredField("number", pageNumberShape),
	response.RequiredField("size", pageSizeShape),
	response.RequiredField("tasks", response.ArrayOf(taskResponse)),
)

var pageEnvelope = response.Envelope(
	response.RequiredField("page", pageShape),
)

response.RenderJSON(r, http.StatusOK, pageEnvelope.Body(
	response.Field("page", response.Fields(
		response.Field("number", pageNumber(page.Number)),
		response.Field("size", pageSize(page.Size)),
		response.Field("tasks", page.Tasks),
	)),
))
```

## Endpoint Definitions

Use `endpoint.DefineEndpoint(endpoint.EndpointSpec{...})` for new endpoints.
The struct keeps the endpoint contract in one readable place.

```go
var CreateTask = endpoint.DefineEndpoint(endpoint.EndpointSpec{
	Method: endpoint.POST,
	Path:   "/tasks/create",
	Aliases: []endpoint.AliasSpec{
		endpoint.Alias("/tasks/new"),
	},
	Handler: func(r *endpoint.Req) {
		params, err := createTaskRequest.Parse(r.Body, param.WithRequestCaller(r))
		if err != nil {
			renderParamError(r, err)
			return
		}

		task, appErr := service.CreateTask(r.Req.Context(), params)
		if appErr != nil {
			response.RenderErr(r, appErr)
			return
		}

		response.RenderJSON(r, http.StatusCreated, taskResponse.Body(task))
	},
	Access: endpoint.EndpointAccessSpec{
		Authorization: endpoint.RequiredAuthorization(endpoint.AuthorizationKindBearer),
		Callers:       []caller.Caller{PublicAPI, Dashboard},
	},
	Idempotency: endpoint.EndpointIdempotencySpec{
		Enabled: true,
	},
	Priority: endpoint.PriorityHigh,
	Timeout: endpoint.EndpointTimeoutSpec{
		ReadBody: 2 * time.Second,
		Handler:  10 * time.Second,
		Write:    2 * time.Second,
	},
	Limits: endpoint.EndpointLimitsSpec{
		MaxRequestBytes: 64 << 10,
	},
	Operation: endpoint.OperationSpec{
		ID:      "create_task",
		Summary: "Create task",
		Accounting: endpoint.AccountingSpec{
			Cost: endpoint.CostAccountingEnabled,
		},
	},
	Route: endpoint.RouteSpec{
		Backend: endpoint.RouteBackend{
			Address:  "https://tasks.example.internal",
			PathMode: endpoint.RoutePathModeAppend,
			Timeout:  30 * time.Second,
		},
	},
})
```

Prefer endpoint-level `Timeout`, `Limits`, `Priority`, `Access`, `Operation`,
and `Route` metadata over service-local side tables. `Operation.ID` is the
shared operation identity for OpenAPI, generated docs, completion events, and
cost accounting; when it is empty, completion cost events fall back to
`METHOD pattern`. `Route` is routing/backend metadata only. Transcribers can
only produce complete documents when the endpoint contract carries the relevant
metadata.

Use aliases when an existing endpoint needs an additional route while keeping
one handler and one endpoint contract. The canonical route keeps
`Operation.ID`; alias routes get `<operation_id>Alias` in generated documents
unless the alias sets an explicit operation ID:

```go
var CreateTask = endpoint.DefineEndpoint(endpoint.EndpointSpec{
	Method: endpoint.POST,
	Path:   "/tasks/create",
	Aliases: []endpoint.AliasSpec{
		endpoint.Alias("/tasks/new"),
		endpoint.Alias("/tasks/legacy_create").WithOperationID("legacyCreateTask"),
	},
	Handler: createTask,
	Operation: endpoint.OperationSpec{
		ID: "createTask",
	},
})
```

Aliases are route-level metadata only. They share the endpoint's request,
response, auth, caller availability, idempotency, timeout, limit, accounting,
and backend route behavior.

`NewEndpoint`, `NewIdempotentEndpoint`, and
`NewIdempotentEndpointWithScopeResolver` remain for compatibility. New code
should prefer `DefineEndpoint`.

## Endpoint Groups

Use endpoint groups for shared defaults, then mount them on httpapi's default
mux:

```go
group := endpoint.EndpointGroup{
	PathPrefix: "/v1",
	Operation: endpoint.OperationSpec{
		Summary: "Task endpoint",
		Accounting: endpoint.AccountingSpec{
			Cost: endpoint.CostAccountingEnabled,
		},
	},
	Route: endpoint.RouteSpec{
		Backend: endpoint.RouteBackend{
			Address: "https://tasks.example.internal",
		},
	},
	Timeout: endpoint.EndpointTimeoutSpec{
		Handler: 5 * time.Second,
	},
	Endpoints: []endpoint.Endpoint{
		CreateTask,
	},
}

group.RequireAuthorization(endpoint.AuthorizationKindBearer)
group.AvailableTo(PublicAPI, Dashboard)

mux := endpoint.NewMux()
mux.MustMount(group)

srv := server.New(server.Config{
	Mux: mux,
})
srv.ListenAndServe()
```

`endpoint.Mux` implements `http.Handler`, wraps Go's standard `http.ServeMux`,
keeps a mounted endpoint registry for documentation and operational tooling, and
rejects duplicate method/path registrations before mutating the mux.

When an application already owns a standard library mux, pass it in and still
use httpapi's mounting semantics:

```go
stdlibMux := http.NewServeMux()
apiMux := endpoint.NewMux(endpoint.WithServeMux(stdlibMux))
apiMux.MustMount(group)
```

Use `Mount(...) error` when startup code wants explicit error handling, and
`MustMount(...)` when a bad route definition should fail fast during boot.

Group availability narrows endpoint availability; it does not widen a restricted
endpoint. An unrestricted group or endpoint is available to all callers.
`EndpointGroup.Operation` defaults inherit summary and accounting only;
`Operation.ID` never inherits because it must be unique for OpenAPI, generated
docs, completion events, and accounting.

## Server

The `server` package is the default way to turn a mux into an `http.Server`:

```go
mux := endpoint.NewMux()
mux.MustMount(group)

srv := server.New(server.Config{
	Port: "8080",
	Mux:  mux,
	CORS: server.PermissiveCORS(),
	Middleware: []server.Middleware{
		traceMiddleware,
		authContextMiddleware,
	},
})

srv.ListenAndServe()
```

`server.New` returns `server.Server`, which embeds `net/http.Server` and keeps
the configured `endpoint.Mux` available for whole-server documentation.

Configured CORS runs before the middleware chain, so browser preflight requests
can complete without entering auth, idempotency, or service middleware. Actual
requests then enter middleware in declaration order.

Use a custom CORS policy when the API should not be public to every browser
origin:

```go
srv := server.New(server.Config{
	Mux: mux,
	CORS: &server.CORSConfig{
		AllowedOrigins: []string{"https://dashboard.example"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost},
		AllowedHeaders: []string{"authorization", "content-type"},
		ExposeHeaders:  []string{"x-request-id"},
		MaxAge:         10 * time.Minute,
	},
})
```

Use `server.CORS(...)` or `server.CORSMiddleware(...)` in `Middleware` only
when an application intentionally needs custom ordering.

Applications still own service-specific setup: authentication providers,
runtime adapters, CORS policy choice, observability exporters, and graceful
shutdown. Use `server.Config.Handler` only when an application needs a fully
custom handler tree instead of the `endpoint.Mux` path.

Add `server.Config.Description` when the server should generate API documents:

```go
srv := server.New(server.Config{
	Mux: mux,
	Description: server.Description{
		Title:       "Tasks API",
		Description: "Task management API.",
		Version:     "2026-07-28",
		PathPrefix: "/v1",
		PublicURLs: []server.PublicURL{{
			URL:         "https://api.example.com",
			Description: "Production",
		}},
		GatewayHost: "api.example.gateway.dev",
		DefaultBackend: endpoint.RouteBackend{
			Address: "https://tasks.example.run.app",
			PathMode: endpoint.RoutePathModeAppend,
			Timeout: 15 * time.Second,
		},
	},
})
```

The listener address and the public API URLs are intentionally separate.
`Config.Addr`, `Host`, and `Port` describe where the process binds; `Description`
describes the API contract clients and gateways should see.

## Service Wiring

Applications own the real authentication, audit, and idempotency systems.
`httpapi` provides injection points.

Configure authentication:

```go
restore := request.ConfigureAuthenticator(request.AuthenticatorFunc(
	func(ctx context.Context, req *request.Req, auth request.AuthenticationRequest) (*request.Session, error) {
		// Call your auth boundary here, then return a session.
		return &request.Session{
			ID:       "sess_...",
			App:      request.App{ID: "app_..."},
			AuthMode: auth.Type,
		}, nil
	},
))
defer restore()
```

Configure auth scheme names when your API uses non-default authorization
prefixes:

```go
restore := request.ConfigureAuthorizationSchemes(request.AuthorizationSchemes{
	Service: "Internal-Service",
})
defer restore()
```

Configure request audit persistence:

```go
restore := endpoint.ConfigureAuditSink(endpoint.AuditSinkFunc(
	func(ctx context.Context, req *endpoint.Req) error {
		return auditStore.Save(ctx, req)
	},
))
defer restore()
```

Configure idempotency before mounting idempotent endpoints:

```go
restoreStore := endpoint.ConfigureIdempotencyStore(store)
defer restoreStore()

restoreNamespace := endpoint.ConfigureIdempotencyScopeNamespace("tasks")
defer restoreNamespace()
```

Idempotent endpoints fail closed with `idempotency_storage_unavailable` until a
store is configured.

## Transcription

Endpoint metadata is provider-neutral. Target packages decide how to translate
that metadata.

Prefer describing the configured server when writing documents for a complete
API surface:

```go
publicDoc, err := srv.DescribeOpenAPI31()
if err != nil {
	return err
}

gatewayDoc, err := srv.DescribeGCPAPIGateway()
if err != nil {
	return err
}
```

Direct transcribers remain useful for tests and tooling that intentionally works
with one endpoint group instead of the whole server:

```go
doc, err := openapi31.Transcriber{
	Info: spec.Info{
		Title:   "Tasks API",
		Version: "2026-07-28",
	},
	Servers: []spec.Server{{URL: "https://api.example.com"}},
}.TranscribeGroupDocument(group)
```

Shared OpenAPI document shapes live in `openapi/spec`. Target-specific packages
should be added under `openapi/<target>` and should translate from endpoint and
response metadata rather than adding provider fields to endpoint structs.

## Best Practices

- Define callers once and reuse the values everywhere.
- Put endpoint requirements on `EndpointSpec`, not on `request.Req`.
- Parse request bodies once with `param`; do not add a second validation phase.
- Return domain-ready params from parser boundaries.
- Treat unavailable restricted params as unexpected params.
- Use response shapes for public contracts and caller-sensitive attributes.
- Use `response.Project` when a domain value needs a response view.
- Keep service-specific auth, audit, storage, logging, and configuration behind
  the injected interfaces.
- Prefer provider-neutral route metadata; let transcribers speak provider
  dialects.
- Make timeout, request-size limit, and priority choices explicit on endpoints
  when they matter.

## Package Boundaries

The root package is doc-only. Application code should import subpackages:

- `github.com/zebodotdev/httpapi/auth`
- `github.com/zebodotdev/httpapi/caller`
- `github.com/zebodotdev/httpapi/endpoint`
- `github.com/zebodotdev/httpapi/erreur`
- `github.com/zebodotdev/httpapi/param`
- `github.com/zebodotdev/httpapi/request`
- `github.com/zebodotdev/httpapi/response`
- `github.com/zebodotdev/httpapi/server`
- `github.com/zebodotdev/httpapi/openapi/openapi31`
- `github.com/zebodotdev/httpapi/openapi/gcpapigateway`
- `github.com/zebodotdev/httpapi/openapi/spec`
