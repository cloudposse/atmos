# Pact Interaction Contracts: Atmos → AtmosPro

**Consumer**: `atmos` (Atmos CLI, `pkg/pro/AtmosProAPIClient`)
**Provider**: `AtmosPro` (Atmos Pro backend API)
**Pact Spec Version**: 2.0.0
**Generated file**: `pacts/atmos-AtmosPro.json`

---

## Interaction 1: UploadAffectedStacks

```
State:       "workspace exists and accepts affected stacks"
Description: "a request to upload affected stacks"

Request:
  Method:  POST
  Path:    /api/v1/affected-stacks
  Headers:
    Authorization:  Like("Bearer test-token")
    Content-Type:   "application/json"
  Body:
    {
      "head_sha":   Like("abc123def456"),
      "base_sha":   Like("111222333444"),
      "repo_url":   Like("https://github.com/org/repo"),
      "repo_name":  Like("repo"),
      "repo_owner": Like("org"),
      "repo_host":  Like("github.com"),
      "stacks":     EachLike({
        "component": Like("vpc"),
        "stack":     Like("dev-us-east-1")
      }, min: 0)
    }

Response:
  Status: 200
  Body:
    { "success": true }
```

---

## Interaction 2: LockStack

```
State:       "workspace exists and stack is unlocked"
Description: "a request to lock a stack"

Request:
  Method:  POST
  Path:    /api/v1/locks
  Headers:
    Authorization: Like("Bearer test-token")
    Content-Type:  "application/json"
  Body:
    {
      "key": Like("org/repo/dev/vpc"),
      "ttl": Like(3600)
    }

Response:
  Status: 200
  Body:
    {
      "success": true,
      "data": {
        "id":          Like("lock-id-uuid"),
        "key":         Like("org/repo/dev/vpc"),
        "expiresAt":   Term(matcher: "^\\d{4}-\\d{2}-\\d{2}T", generate: "2026-06-09T10:00:00Z")
      }
    }
```

---

## Interaction 3: UnlockStack

```
State:       "workspace exists and stack is locked"
Description: "a request to unlock a stack"

Request:
  Method:  DELETE
  Path:    /api/v1/locks
  Headers:
    Authorization: Like("Bearer test-token")
    Content-Type:  "application/json"
  Body:
    {
      "key": Like("org/repo/dev/vpc")
    }

Response:
  Status: 200
  Body:
    {
      "success": true,
      "data": {}
    }
```

---

## Interaction 4: GetGitHubOIDCToken

```
State:       "GitHub Actions OIDC endpoint is available"
Description: "a request to retrieve a GitHub OIDC token"

Note: This interaction uses a separate TLS mock provider (second MockProvider instance).
      The test sets oidcHTTPClientOverride to the pact TLS mock client.

Request:
  Method:  GET
  Path:    /token
  Query:   audience=atmos-pro.com
  Headers:
    Authorization: Like("Bearer test-request-token")

Response:
  Status: 200
  Headers:
    Content-Type: "application/json"
  Body:
    {
      "value": Like("eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...")
    }
```

---

## Interaction 5: ExchangeOIDCToken

```
State:       "OIDC token is valid and workspace exists"
Description: "a request to exchange a GitHub OIDC token for an Atmos Pro token"

Request:
  Method:  POST
  Path:    /api/v1/auth/github-oidc
  Headers:
    Content-Type: "application/json"
    User-Agent:   Term(matcher: "^atmos/", generate: "atmos/1.0.0 (linux; amd64)")
  Body:
    {
      "token":       Like("eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."),
      "workspaceId": Like("workspace-uuid-1234")
    }

Response:
  Status: 200
  Body:
    {
      "success": true,
      "data": {
        "token": Like("atmos-jwt-token-string")
      }
    }
```

---

## Interaction 6: UploadInstances

```
State:       "workspace exists and accepts drift detection instances"
Description: "a request to upload drift detection instances"

Request:
  Method:  POST
  Path:    /api/v1/instances
  Headers:
    Authorization: Like("Bearer test-token")
    Content-Type:  "application/json"
  Body:
    {
      "repo_url":   Like("https://github.com/org/repo"),
      "repo_name":  Like("repo"),
      "repo_owner": Like("org"),
      "repo_host":  Like("github.com"),
      "instances":  EachLike({
        "component":      Like("vpc"),
        "stack":          Like("dev-us-east-1"),
        "component_type": Like("terraform")
      }, min: 1)
    }

Response:
  Status: 200
  Body:
    { "success": true }
```

---

## Interaction 7: UploadInstanceStatus

```
State:       "workspace exists and instance exists for owner/repo"
Description: "a request to upload instance drift status"

Request:
  Method:  PATCH
  Path:    /api/v1/repos/org/repo/instances
  Query:   stack=dev-us-east-1&component=vpc
  Headers:
    Authorization: Like("Bearer test-token")
    Content-Type:  "application/json"
  Body:
    {
      "command":   Like("terraform plan"),
      "exit_code": Like(0)
    }

Response:
  Status: 200
  Body:
    { "success": true }
```

---

## Interaction 8: CreateCommit

```
State:       "workspace exists and GitHub App is authorized"
Description: "a request to create a commit via Atmos Pro GitHub App"

Request:
  Method:  POST
  Path:    /api/v1/git/commit
  Headers:
    Authorization: Like("Bearer test-token")
    Content-Type:  "application/json"
  Body:
    (per dtos.CommitRequest — to be confirmed during implementation)

Response:
  Status: 200
  Body:
    {
      "success": true,
      "data": {
        "sha": Like("abc123def456789")
      }
    }
```

---

## Notes

- `Like(value)` — validates the type and nesting structure; the exact runtime value may differ.
- `EachLike(obj, min: N)` — validates each element of an array matches the given shape.
- `Term(matcher, generate)` — validates the value against the regex; `generate` is used in the mock response.
- Path parameters in Interaction 7 use URL-encoded values; pact V2 path matching uses string equality on the full path.
- Interaction 4 (GitHub OIDC) uses a separate `NewV2Pact` instance configured for TLS
  because the production client enforces `https://` scheme via `buildOIDCRequestURL`.
