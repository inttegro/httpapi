# HTTPAPI transcription profiles

Transcription profiles define interoperable mappings from the portable HTTPAPI
contract into a particular machine-readable target. Profiles are optional and
versioned independently from the core HTTPAPI specification.

| Profile | Version | Requires | Document |
| --- | --- | --- | --- |
| OpenAPI 3.1 | 0.1 | HTTPAPI 0.2 | [OpenAPI 3.1 Transcription Profile 0.1](openapi-3.1/v0.1.md) |

An implementation can conform to HTTPAPI without implementing any
transcription profile. A transcription implementation claiming a profile MUST
name both the core HTTPAPI version and the profile version.
