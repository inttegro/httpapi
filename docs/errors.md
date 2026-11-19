---
title: Return errors clients can act on
sidebarTitle: Errors
description: Use stable codes for client behavior and keep internal failure detail out of the response.
---

HTTPAPI renders one structured error model across request parsing, access checks, runtime failures, and application errors. Clients should make decisions from stable fields, not from message text.

## Map domain failures deliberately

Return an `*erreur.Error` through `response.Error`:

```go
func lookupTask(_ *endpoint.Req) *response.Res {
	task, found := findTask("task_123")
	if !found {
		return response.Error(erreur.NotFound(
			"task_not_found",
			"task was not found",
			"check the task id and try again.",
		))
	}

	return response.JSON(http.StatusOK, task)
}
```

Choose a constructor that matches the failure: `Unauthenticated`, `Forbidden`, `InvalidParam`, `NotFound`, `Conflict`, `Transient`, or another explicit category. Use `Unexpected` only when the service cannot classify the failure safely.

## Treat codes as the compatibility surface

The standard error object can include:

| Field | Client use |
| --- | --- |
| `code` | Exact machine-readable condition |
| `type` | Broad handling category |
| `cause` | Origin of the failure |
| `fix_code` | Suggested remediation class |
| `message` | Short human-readable summary |
| `detail` | Safe, actionable context |
| `url` | Error reference |

Keep `code` stable. Human-readable text can improve without forcing client releases.

## Let the runtime own boundary failures

You do not need to recreate errors for malformed JSON, invalid parameters, unsupported methods or media types, request limits, missing authorization, unavailable callers, idempotency conflicts, or timeouts. The endpoint runtime rejects those conditions before or around the handler and renders the standard envelope.

`DefineJSONEndpoint` also converts `param.Error` values automatically. Application handlers should only map failures from domain work they actually perform.

## Preserve causes internally

Keep the original Go error in logs, traces, or an internal wrapper. Do not copy database messages, provider payloads, stack traces, or arbitrary error strings into public `message` or `detail` fields.

<Warning>
  Returning a raw error string can leak credentials, internal identifiers, query text, or provider behavior. Construct the public error explicitly and record the private cause separately.
</Warning>
