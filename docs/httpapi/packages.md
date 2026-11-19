---
title: Go package map
sidebarTitle: Packages
description: Import the package that owns the HTTP behavior you need.
---

The module root is documentation-only. Runtime behavior lives in focused packages.

| Package | Use it for |
| --- | --- |
| `auth` | Provider-neutral sessions, authentication requests, and authorization schemes |
| `caller` | Trusted caller labels, caller sets, and directional projection policies |
| `cost` | Provider-neutral operation usage records and sinks |
| `edge` | Provider-discriminated request evidence contracts |
| `edge/cloudflare` | Trusted Cloudflare request-evidence extraction |
| `endpoint` | Endpoint contracts, muxes, runtime enforcement, audit, completion, idempotency, controls, and availability |
| `erreur` | Structured public errors and remediation metadata |
| `param` | Typed JSON request shapes, normalization, presence rules, and parse errors |
| `request` | Safe incoming requests, sessions, callers, request metadata, and redacted audit output |
| `response` | Response shapes, caller-aware projection, envelopes, constructors, and writing |
| `server` | `http.Server` construction, middleware, CORS, and whole-server descriptions |
| `openapi/openapi31` | Public OpenAPI 3.1 transcription |
| `openapi/gcpapigateway` | Google Cloud API Gateway Swagger 2.0 transcription |
| `openapi/spec` | Shared API-description document structures |

Most services begin with `endpoint`, `param`, `response`, and `server`, then add `request` and `caller` when authentication or caller-specific behavior is required.

Use [pkg.go.dev](https://pkg.go.dev/github.com/inttegro/httpapi) for exported Go symbols. Use these guides for integration decisions, and the [versioned specification](/httpapi/specification) for portable behavior.
