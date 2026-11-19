---
title: Make writes safe to retry
sidebarTitle: Idempotency
description: Reserve an idempotency key before business work and replay only completed successful responses.
---

Enable idempotency for a write when retrying the same intended operation must not repeat its effect.

## Enable it on the endpoint

```go
Idempotency: endpoint.EndpointIdempotencySpec{
	Enabled: true,
},
```

Clients may send the key in the `Idempotency-Key` header or in `request_meta.idempotency_key`. If both are present, their normalized values must match.

## Install durable storage before serving

```go
restoreStore := endpoint.ConfigureIdempotencyStore(store)
defer restoreStore()

restoreNamespace := endpoint.ConfigureIdempotencyScopeNamespace("tasks")
defer restoreNamespace()
```

The store must atomically reserve a `(scope, key)` pair, complete it with the replayable response, and release incomplete reservations. Implement `endpoint.IdempotencyStore` against storage shared by every instance of the service.

Idempotent endpoints fail closed with `idempotency_storage_unavailable` when a reservation is required and no usable store is configured. This prevents the runtime from executing a write while falsely claiming replay safety.

## Understand the default scope

For an authenticated request, the default scope combines:

```text
application-id#service-namespace#METHOD#route-pattern
```

The service namespace prevents unrelated binaries that share a store from colliding. The application, method, and route keep a client key local to one operation boundary.

<Warning>
  An unauthenticated request without a custom scope resolver bypasses idempotency because HTTPAPI cannot derive a safe application scope. Supply a trustworthy `ScopeResolver` before relying on idempotency for anonymous writes.
</Warning>

## Know what gets replayed

HTTPAPI stores and replays only non-streaming `2xx` responses. A replay returns the stored status, content type, replayable headers, and body with `Idempotent-Replayed: true`.

- A matching completed request is replayed.
- A matching reservation still in progress returns an in-progress error.
- Reusing the key with another request fingerprint returns a conflict.
- A failed, missing, or streaming response releases the reservation.

The fingerprint is calculated from canonical request JSON after removing top-level `request_meta`. A retry may therefore change its idempotency-key location or response projection while remaining the same operation.

<Note>
  Keep records for at least the 24-hour HTTPAPI retention window. Clients must reuse the same key for the same intended operation and use a new key for a new operation.
</Note>
