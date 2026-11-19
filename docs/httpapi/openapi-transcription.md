---
title: Publish OpenAPI from the running contract
sidebarTitle: OpenAPI
description: Generate an OpenAPI 3.1 document from the endpoint groups mounted by your server.
---

Describe the public API on `server.Config`, then transcribe the same mounted contracts the runtime serves. Do not maintain a second response schema by hand.

## Add the public description

```go
srv := server.New(server.Config{
	Mux: mux,
	Description: server.Description{
		Title:       "Tasks API",
		Description: "Create and manage tasks.",
		Version:     "2026-11-19",
		PathPrefix:  "/v1",
		PublicURLs: []server.PublicURL{
			{
				URL:         "https://api.example.com",
				Description: "Production",
			},
		},
	},
})
```

The listener address is intentionally separate. `Port` tells the process where to bind; `PublicURLs` tells clients where the API is reachable.

## Generate the document

```go
document, err := srv.DescribeOpenAPI31()
if err != nil {
	return fmt.Errorf("describe OpenAPI: %w", err)
}

body, err := json.MarshalIndent(document, "", "  ")
if err != nil {
	return fmt.Errorf("encode OpenAPI: %w", err)
}

if err := os.WriteFile("openapi.json", body, 0o644); err != nil {
	return fmt.Errorf("write OpenAPI: %w", err)
}
```

The Go transcriber currently emits OpenAPI 3.1.1. It reads routes, aliases, request and response contracts, authorization requirements, priorities, operation kinds, and the public server description. Internal endpoints are omitted.

## Declare the transcription limit

The current OpenAPI 3.1 transcriber aggregates attributes across caller-specific response views. The document therefore describes the set of possible public fields, but it does not encode which caller may receive each field.

Runtime response projection remains the authority boundary. Do not use the generated OpenAPI document to make authorization decisions.

## Use the right target

`srv.DescribeGCPAPIGateway()` emits a Swagger 2.0 document for Google Cloud API Gateway and includes routing metadata required by that deployment target. Direct transcribers in `openapi/openapi31` and `openapi/gcpapigateway` are useful for tests or tools that intentionally work with one endpoint group.

The language-independent specification defines transcription behavior separately from target profiles. See the [OpenAPI 3.1 profile](/httpapi/profiles/openapi-3.1-v0.1) when implementing or assessing portable conformance.
