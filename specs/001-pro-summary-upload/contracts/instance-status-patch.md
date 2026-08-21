# Contract: Instance Status PATCH Endpoint

**Endpoint**: `PATCH /api/v1/repos/{owner}/{repo}/instances?stack={stack}&component={component}`

**Authentication**: Bearer token (Atmos Pro API token or OIDC exchange)

**Retry**: Up to 3 attempts with exponential backoff (initial delay 1s); 401 triggers token
refresh before retry

---

## Request

### Headers

```
Content-Type: application/json
Authorization: Bearer <token>
```

### Path Parameters

| Parameter | Description |
|---|---|
| `owner` | Repository owner (URL-path-escaped) |
| `repo` | Repository name (URL-path-escaped) |

### Query Parameters

| Parameter | Description |
|---|---|
| `stack` | Stack name (URL-query-escaped) |
| `component` | Component name (URL-query-escaped) |

### Payload — Existing Fields (unchanged)

```json
{
  "command":   "plan" | "apply" | "deploy",
  "exit_code": <integer>,
  "last_run":  "<ISO 8601 datetime>"
}
```

### Payload — Extended (when `ci.enabled` and `--upload-status` and terraform)

**Plan with changes:**

```json
{
  "command":        "plan",
  "exit_code":      2,
  "last_run":       "2026-06-09T10:00:00Z",
  "component_type": "terraform",
  "metadata": {
    "has_changes":     true,
    "has_errors":      false,
    "warnings":        ["Warning: Value for undeclared variable..."],
    "errors":          [],
    "resource_counts": {
      "create":  3,
      "change":  1,
      "replace": 0,
      "destroy": 2
    },
    "output_log": "<base64-encoded masked stdout>",
    "truncated":  false
  }
}
```

**Plan with no changes:**

```json
{
  "command":        "plan",
  "exit_code":      0,
  "last_run":       "2026-06-09T10:00:00Z",
  "component_type": "terraform",
  "metadata": {
    "has_changes":     false,
    "has_errors":      false,
    "warnings":        [],
    "errors":          [],
    "resource_counts": {
      "create":  0,
      "change":  0,
      "replace": 0,
      "destroy": 0
    },
    "output_log": "<base64-encoded masked stdout>"
  }
}
```

**Apply with outputs:**

```json
{
  "command":        "apply",
  "exit_code":      0,
  "last_run":       "2026-06-09T10:05:00Z",
  "component_type": "terraform",
  "metadata": {
    "has_changes": false,
    "has_errors":  false,
    "warnings":    [],
    "errors":      [],
    "outputs": {
      "vpc_id":     "vpc-abc123",
      "secret_key": "<MASKED>"
    },
    "output_log": "<base64-encoded masked stdout>"
  }
}
```

**Deploy (command field preserves "deploy"):**

```json
{
  "command":        "deploy",
  "exit_code":      0,
  "last_run":       "2026-06-09T10:05:00Z",
  "component_type": "terraform",
  "metadata": { ... }
}
```

**Truncated output log:**

```json
{
  "metadata": {
    "output_log": "<base64-encoded tail of masked stdout>",
    "truncated":  true,
    ...
  }
}
```

**Non-terraform or gating conditions false (backward-compatible — no new fields):**

```json
{
  "command":   "sync",
  "exit_code": 0,
  "last_run":  "2026-06-09T10:00:00Z"
}
```

---

## Response

| Status | Meaning |
|---|---|
| 200 OK | Upload accepted |
| 401 Unauthorized | Token invalid or expired — CLI retries after token refresh |
| 5xx | Server error — CLI retries with backoff |

Response body is not used by the CLI.

---

## Server Compatibility

The server MUST handle payloads with or without `component_type` and `metadata` to maintain
backward compatibility with older CLI versions that do not send these fields.

---

## `StatusDataProvider` Interface Contract

Implemented by CI plugins that want to contribute metadata to the upload:

```go
type StatusDataProvider interface {
    BuildStatusData(output string, command string) map[string]any
}
```

- `output`: masked stdout from the command (single buffer)
- `command`: the subcommand (`"plan"`, `"apply"`, `"deploy"`)
- Returns: metadata map, or nil if the plugin has nothing to contribute
- MUST NOT return a map containing `"output_log"` or `"truncated"` — those keys are set by
  the upload caller after size-limit enforcement
- MUST replace sensitive output values with `"<MASKED>"` before returning
