---
title: HTTPAPI specification
sidebarTitle: Overview
description: Read the language-independent contract, conformance model, and versioned transcription profiles.
---

Use the specification when implementing HTTPAPI outside Go, assessing conformance, or resolving a question about observable behavior. Use the guides when integrating the Go reference implementation.

## Published editions

Published editions are immutable. A correction or behavioral change requires a new edition.

| Edition | Published | Source tag |
| --- | --- | --- |
| [HTTPAPI 0.2](/httpapi/specification/v0.2) | 20 October 2026 | [`spec-v0.2.0`](https://github.com/inttegro/httpapi/tree/spec-v0.2.0) |
| [HTTPAPI 0.1](/httpapi/specification/v0.1) | 20 October 2026 | [`spec-v0.1.0`](https://github.com/inttegro/httpapi/tree/spec-v0.1.0) |

Start with 0.2 for new implementations. It defines endpoint and shape contracts, caller authority, request validation, standard request metadata, response projection and shaping, structured errors, runtime order, idempotency, observability, transcription, determinism, and security requirements.

## Choose the conformance you implement

HTTPAPI 0.2 defines three conformance classes:

1. **Contract implementation** — implements shapes, visibility, validation, response projection, envelope rules, and structured errors.
2. **Runtime implementation** — also executes endpoints in the required processing order.
3. **Transcription implementation** — converts contracts into a declared target and profile.

An implementation must state its HTTPAPI edition, claimed classes, supported profiles, and every implementation limit that can reject an otherwise valid contract or request.

## Optional transcription profiles

Profiles are target-specific and versioned independently from the core specification.

| Target | Profile | Requires |
| --- | --- | --- |
| [OpenAPI 3.1](/httpapi/profiles/openapi-3.1-v0.1) | 0.1 | HTTPAPI 0.2 |

The profile defines how portable HTTPAPI semantics map into that target. Supporting core transcription does not automatically mean an implementation conforms to every target profile.
