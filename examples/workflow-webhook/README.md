# Webhook Step Type

This example demonstrates the `webhook` workflow step type, which performs an HTTP
request with a configurable method/verb, query-string parameters, headers, and a request
body (raw or form/JSON). Requests get per-attempt timeouts and retries that compose with
the step's `retry:` policy.

## Workflows

- **`notify`** — `POST`s a JSON payload to `WEBHOOK_URL`, retrying transient failures
  (`5xx`, `429`, network errors) with exponential backoff, then prints the status code.
- **`poll-health`** — `GET`s `HEALTH_URL` and retries until the response body matches a
  "healthy" pattern.

## Run It

Point the workflows at any reachable endpoint via environment variables:

```shell
# Notify an endpoint (use your own URL, or a request-bin style service).
WEBHOOK_URL=https://example.com/hook atmos workflow notify -f webhook

# Poll a health endpoint until it reports healthy.
HEALTH_URL=https://example.com/healthz atmos workflow poll-health -f webhook
```

## Key Fields

| Field           | Description                                                                 |
|-----------------|-----------------------------------------------------------------------------|
| `url`           | Request URL (required, supports templates).                                 |
| `method`        | HTTP verb: `GET` (default), `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`.       |
| `query`         | Query-string parameters.                                                    |
| `headers`       | Request headers.                                                            |
| `body` / `form` | Raw body, or key-value params (urlencoded, or JSON when `Content-Type` is JSON). Mutually exclusive. |
| `expect.status` | Acceptable status codes (default: any `2xx`).                               |
| `expect.response` | Regexes the response body must match (at least one).                      |
| `timeout`       | Per-attempt timeout (default `30s`).                                        |
| `retry`         | Retry policy; transport errors / `5xx` / `429` retry by default.            |

The response is available to later steps as `{{ .steps.<name>.value }}` (body) and
`{{ .steps.<name>.metadata.status_code }}`.
