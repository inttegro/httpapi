# HTTPAPI specifications

Each published HTTPAPI specification is retained as a separate document. A
published edition is immutable: corrections or behavioral changes produce a new
version instead of rewriting the document that existing implementations target.

| Version | Published | Document | Source tag |
| --- | --- | --- | --- |
| 0.2 | 20 October 2026 | [HTTPAPI Specification 0.2](v0.2.md) | [`spec-v0.2.0`](https://github.com/inttegro/httpapi/tree/spec-v0.2.0) |
| 0.1 | 20 October 2026 | [HTTPAPI Specification 0.1](v0.1.md) | [`spec-v0.1.0`](https://github.com/inttegro/httpapi/tree/spec-v0.1.0) |

The repository README documents the Go reference implementation. These
documents define the language-neutral contract.

## Optional transcription profiles

Profiles define mappings into specific machine-readable targets without making
those targets part of core HTTPAPI conformance. Profiles have their own version
history.

| Profile | Version | Requires | Document |
| --- | --- | --- | --- |
| OpenAPI 3.1 | 0.1 | HTTPAPI 0.2 | [OpenAPI 3.1 Transcription Profile 0.1](profiles/openapi-3.1/v0.1.md) |

See the [transcription profile index](profiles/README.md) for profile
conformance rules.
