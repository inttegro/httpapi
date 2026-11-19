---
title: Why HTTPAPI exists
sidebarTitle: Why HTTPAPI
description: Stop maintaining separate definitions for routing, validation, authorization, response shaping, and OpenAPI.
---

Most HTTP services describe one endpoint several times. The router knows the path, validation code knows the payload, authorization code knows the caller, and an OpenAPI file tries to repeat all of it. Those copies eventually disagree.

HTTPAPI makes the executable endpoint contract the shared source.

## Replace scattered policy with one contract

| Concern | Without a shared contract | With HTTPAPI |
| --- | --- | --- |
| Input | Decode JSON, then remember to validate it | Parse once into a complete Go value |
| Access | Repeat checks inside handlers | Declare authorization and callers on the endpoint |
| Output | Build maps and remove sensitive fields manually | Project a declared shape for the active caller |
| Runtime safety | Keep limits, timeouts, and idempotency in side tables | Store them beside the endpoint |
| API description | Maintain a separate OpenAPI document | Transcribe the mounted contracts |

The result is not less domain code. It is less duplicated HTTP policy around that code.

## Keep business work outside HTTPAPI

HTTPAPI owns the boundary around a handler. It can establish that a request is well-formed, authenticated, available to its caller, within limits, and safe to execute once. It does not decide whether a customer owns a resource or whether an order may change state.

Those decisions still belong to the domain service, where current application state and transactions are available.

## Treat OpenAPI as an output

OpenAPI is useful for client generation and API discovery, but it cannot represent every runtime rule without extensions or loss. HTTPAPI therefore transcribes OpenAPI from the executable contract. OpenAPI does not become the runtime model.

<Tip>
  If you want a Go service to execute and publish the same contract, start with the [quickstart](/quickstart). If you need a language-neutral implementation contract, use the [HTTPAPI specification](/specification).
</Tip>
