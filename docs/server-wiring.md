---
title: Wire HTTPAPI into a service
sidebarTitle: Server setup
description: Install service-owned adapters, mount endpoint groups, and start a conservatively configured HTTP server.
---

HTTPAPI keeps infrastructure behind explicit interfaces. Install the adapters your contracts require before accepting traffic, then mount the endpoints and start the server.

## Install the required adapters

| Contract feature | Startup dependency |
| --- | --- |
| Required authorization | `request.ConfigureAuthenticator` |
| Idempotent endpoints | `endpoint.ConfigureIdempotencyStore` and a service namespace |
| Endpoint controls | `endpoint.ConfigureControlEvaluator` |
| Operational availability | `endpoint.ConfigureAvailabilityEvaluator` |
| Request audit persistence | `endpoint.ConfigureAuditSink` |
| Completion events | `endpoint.ConfigureCompletionSink` |
| Trusted edge evidence | `request.ConfigureEdgeProvider` |

Configuration functions return a restore function. Defer it in tests and temporary overrides. A production process normally installs each adapter once for its lifetime.

```go
restoreAuth := request.ConfigureAuthenticator(authenticator)
defer restoreAuth()

restoreStore := endpoint.ConfigureIdempotencyStore(idempotencyStore)
defer restoreStore()

restoreNamespace := endpoint.ConfigureIdempotencyScopeNamespace("tasks")
defer restoreNamespace()
```

Authentication, idempotency, and endpoint controls fail closed when a contract requires them but their adapter cannot produce a decision. Audit and completion sink failures are recorded without changing the client response.

## Mount endpoint groups

```go
mux := endpoint.NewMux()
if err := mux.Mount(tasksGroup, healthGroup); err != nil {
	return fmt.Errorf("mount HTTPAPI endpoints: %w", err)
}
```

`endpoint.Mux` wraps `http.ServeMux`, rejects duplicate method-and-path pairs before registering the batch, and retains mounted contracts for catalogs and transcription. Use `MustMount` when invalid route configuration should panic during startup.

## Start the server

```go
srv := server.New(server.Config{
	Port: server.PortFromEnv("PORT", server.DefaultPort),
	Mux:  mux,
	CORS: &server.CORSConfig{
		AllowedOrigins: []string{"https://dashboard.example"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost},
		AllowedHeaders: []string{"authorization", "content-type"},
	},
	Middleware: []server.Middleware{
		traceMiddleware,
		trustedCallerMiddleware,
	},
})
```

CORS is the outer boundary, so browser preflight can finish without entering authentication or idempotency. Actual requests enter middleware in declaration order and then reach the HTTPAPI mux.

The server applies bounded defaults for header reads, full request reads, response writes, idle connections, and header size. Override them when measured traffic requires different budgets. Add graceful shutdown in the application process; HTTPAPI does not own process lifecycle.

## Separate the listener from the public API

`Addr`, `Host`, and `Port` control where the process listens. `server.Description` defines what clients and generated gateway documents see: public URLs, version, path prefix, contact details, and default backend routing.

Continue with [OpenAPI transcription](/openapi-transcription) after the mounted server has a public description.
