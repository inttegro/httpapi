---
title: Establish caller authority
sidebarTitle: Callers
description: Attach trusted caller labels once, then reuse them for endpoint access, request parameters, and response fields.
---

A caller is an application-defined label for an already trusted request source. It is not a credential, role string from the client, or replacement for authentication.

## Define stable caller labels

```go
var (
	PublicAPI = caller.Define("public-api")
	Worker    = caller.Define("worker")
	Dashboard = caller.Define("dashboard")
)
```

Reuse these values in endpoint, parameter, and response contracts. Do not recreate caller labels from an unverified header, query parameter, or request body.

## Attach the caller at a trusted boundary

If trusted middleware has already identified the request source, attach it before the HTTPAPI mux runs:

```go
func withPublicAPICaller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := request.ContextWithCaller(r.Context(), PublicAPI)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

Use a fixed label only when that middleware path itself establishes the source, as a dedicated public gateway route would. When HTTPAPI performs credential verification, use the configured request context annotator to map the verified session to a caller.

## Restrict the endpoint

```go
Access: endpoint.EndpointAccessSpec{
	Authorization: endpoint.RequiredAuthorization(
		endpoint.AuthorizationKindBearer,
	),
	Callers: []caller.Caller{PublicAPI, Dashboard},
},
```

An empty caller list is unrestricted. A group may narrow an endpoint's caller set, but it cannot widen a restriction already declared by the endpoint.

Authorization and caller availability answer different questions:

- authorization verifies that the request has an acceptable session kind;
- caller availability decides whether that trusted source may use this contract; and
- domain authorization decides whether the session may act on the specific resource.

## Project downward, never sideways

A worker may sometimes need the exact public representation returned by the API. Declare that direction explicitly:

```go
var responsePolicy = caller.DefineProjectionPolicy(
	caller.Descends(Worker, PublicAPI),
	caller.Descends(Dashboard, PublicAPI),
)

ctx, err := request.ContextWithResponseProjection(
	ctx,
	responsePolicy,
	PublicAPI,
)
```

Projection is a graph, not a numeric privilege level. Identity and declared transitive descent are allowed. Upward or lateral projection returns `caller.ErrProjectionElevation` and leaves the context unchanged.

<Warning>
  Never accept a requested caller and attach it directly. Resolve every requested projection through a `caller.ProjectionPolicy`.
</Warning>
