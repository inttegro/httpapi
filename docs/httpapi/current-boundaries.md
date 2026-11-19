---
title: Current boundaries
sidebarTitle: Boundaries
description: Know what HTTPAPI enforces, what your application must supply, and where the Go reference implementation has declared limits.
---

HTTPAPI owns the executable HTTP contract and the runtime behavior needed to enforce it. It does not replace the rest of your service.

## Your application must supply

- credential verification and identity-provider integration;
- domain authorization and resource-ownership checks;
- databases, transactions, and business workflows;
- shared idempotency storage;
- control and availability evaluators when those features are used;
- audit, completion, logging, metrics, and tracing backends;
- trusted edge configuration; and
- deployment configuration and graceful shutdown.

HTTPAPI supplies interfaces and ordering for these dependencies. Installing a no-op substitute does not satisfy a security or durability requirement.

## Keep these boundaries distinct

- A caller label is trusted authority context, not an authentication scheme.
- Request parsing accepts wire input; it does not validate current database state.
- Response projection controls emitted fields; it does not authorize the resource.
- An endpoint timeout bounds in-process work; a route backend timeout describes generated gateway behavior.
- OpenAPI describes a target projection of the contract; the runtime remains the enforcement boundary.

## Go reference implementation limits

The current implementation:

- serves `GET` and `POST` endpoint methods;
- uses Go `ServeMux` route patterns;
- treats JSON as the default request media type while allowing additional declared media types;
- emits OpenAPI 3.1.1 and Google Cloud API Gateway Swagger 2.0 documents;
- aggregates caller-specific response attributes in its public OpenAPI 3.1 output without preserving per-field caller availability; and
- configures runtime adapters at process scope, so tests and multi-service binaries must install and restore them carefully.

These are implementation limits, not silent claims about the language-independent contract. An implementation that claims conformance must publish its specification edition, conformance classes, optional profiles, and any limit that can reject an otherwise valid contract or request.

## Before production traffic

Confirm that every required adapter is installed, idempotency storage is shared by all instances, caller labels come only from trusted code, request limits are enforced while reading, public errors redact private causes, and generated documents are reviewed for declared transcription loss.

Use [server wiring](/httpapi/server-wiring) for the startup path and [HTTPAPI 0.2](/httpapi/specification/v0.2) for the normative security requirements.
