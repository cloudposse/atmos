# Data Model: Pact Consumer Contract Testing for Atmos Pro API

**Feature**: 001-pact-consumer-contracts
**Date**: 2026-06-09

---

## Core Entities

### PactConsumer

The Atmos CLI client acting as the consumer in all contract interactions.

| Field | Value | Notes |
|-------|-------|-------|
| `Name` | `"atmos"` | Appears in generated pact file name and each interaction |
| `BaseURL` | mock server URL | Injected per test via `MockServerConfig` |
| `APIToken` | `"test-token"` | Static mock value; never a real credential |

### PactProvider

The Atmos Pro backend API acting as the provider in all contract interactions.

| Field | Value | Notes |
|-------|-------|-------|
| `Name` | `"AtmosPro"` | Appears in generated pact file name |
| `BaseURL` | `https://atmos-pro.com` | Production base; overridden to mock in tests |
| `BaseAPIEndpoint` | `api/v1` | Prefixes all paths |

### PactInteraction

A single named request/response pair defined in the consumer tests. There are 8
interactions, one per Atmos Pro API endpoint.

| Field | Type | Description |
|-------|------|-------------|
| `State` | `string` | Provider state precondition (e.g., "workspace exists") |
| `Description` | `string` | Human-readable name used in pact file |
| `Request.Method` | `string` | HTTP method |
| `Request.Path` | `string` | URL path (may include path params) |
| `Request.Headers` | `map[string]string` | Required headers (`Authorization`, `Content-Type`) |
| `Request.Body` | `matchers.S` | pact body matcher tree |
| `Response.Status` | `int` | Expected HTTP status code |
| `Response.Headers` | `map[string]string` | Expected response headers |
| `Response.Body` | `matchers.S` | pact response body matcher tree |

### PactFile

The generated JSON artifact persisted after a successful consumer test run.

| Field | Value | Notes |
|-------|-------|-------|
| `Path` | `pacts/atmos-AtmosPro.json` | Relative to repo root |
| `Consumer` | `{ "name": "atmos" }` | Embedded in file |
| `Provider` | `{ "name": "AtmosPro" }` | Embedded in file |
| `Interactions` | array of 8 | All interactions consolidated into one file |
| `Metadata` | pact spec version | `{ "pactSpecification": { "version": "2.0.0" } }` |

---

## The 8 Interactions

### 1. UploadAffectedStacks

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts affected stacks"` |
| Description | `"a request to upload affected stacks"` |
| Method | `POST` |
| Path | `/api/v1/affected-stacks` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `head_sha` (string), `base_sha` (string), `repo_url` (string), `repo_name` (string), `repo_owner` (string), `repo_host` (string), `stacks` (array) |
| Response Status | `200` |
| Response Body | `{ "success": true }` |

### 2. LockStack

| Field | Value |
|-------|-------|
| State | `"workspace exists and stack is unlocked"` |
| Description | `"a request to lock a stack"` |
| Method | `POST` |
| Path | `/api/v1/locks` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `key` (string), `ttl` (integer), `lockMessage` (string, optional) |
| Response Status | `200` |
| Response Body | `success`, `data.id`, `data.key`, `data.expiresAt` |

### 3. UnlockStack

| Field | Value |
|-------|-------|
| State | `"workspace exists and stack is locked"` |
| Description | `"a request to unlock a stack"` |
| Method | `DELETE` |
| Path | `/api/v1/locks` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `key` (string) |
| Response Status | `200` |
| Response Body | `{ "success": true, "data": {} }` |

### 4. GetGitHubOIDCToken

| Field | Value |
|-------|-------|
| State | `"GitHub Actions OIDC endpoint is available"` |
| Description | `"a request to retrieve a GitHub OIDC token"` |
| Method | `GET` |
| Path | `/token` (mock path representing `ACTIONS_ID_TOKEN_REQUEST_URL`) |
| Request Headers | `Authorization: Bearer <request-token>` |
| Request Query | `audience=atmos-pro.com` |
| Response Status | `200` |
| Response Body | `{ "value": "<oidc-token-string>" }` |

### 5. ExchangeOIDCToken

| Field | Value |
|-------|-------|
| State | `"OIDC token is valid and workspace exists"` |
| Description | `"a request to exchange a GitHub OIDC token for an Atmos Pro token"` |
| Method | `POST` |
| Path | `/api/v1/auth/github-oidc` |
| Request Headers | `Content-Type: application/json` |
| Request Body Fields | `token` (string), `workspaceId` (string) |
| Response Status | `200` |
| Response Body | `success`, `data.token` (string JWT) |

### 6. UploadInstances

| Field | Value |
|-------|-------|
| State | `"workspace exists and accepts instances"` |
| Description | `"a request to upload drift detection instances"` |
| Method | `POST` |
| Path | `/api/v1/instances` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `repo_url`, `repo_name`, `repo_owner`, `repo_host` (strings), `instances` (array of `{component, stack, component_type}`) |
| Response Status | `200` |
| Response Body | `{ "success": true }` |

### 7. UploadInstanceStatus

| Field | Value |
|-------|-------|
| State | `"workspace exists, instance exists for owner/repo"` |
| Description | `"a request to upload instance drift status"` |
| Method | `PATCH` |
| Path | `/api/v1/repos/{owner}/{repo}/instances` |
| Request Query | `stack=<stack>&component=<component>` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | `command` (string), `exit_code` (integer) |
| Response Status | `200` |
| Response Body | `{ "success": true }` |

### 8. CreateCommit

| Field | Value |
|-------|-------|
| State | `"workspace exists and GitHub App is authorized"` |
| Description | `"a request to create a commit via Atmos Pro"` |
| Method | `POST` |
| Path | `/api/v1/git/commit` |
| Request Headers | `Authorization: Bearer <token>`, `Content-Type: application/json` |
| Request Body Fields | per `dtos.CommitRequest` (branch, message, files array) |
| Response Status | `200` |
| Response Body | `success`, `data.sha` (string) |

---

## State Transitions

Pact interactions are stateless from the consumer's perspective — each interaction
describes a single request/response round trip. Provider state strings (the `State`
field) are hints for future provider verification setup; they have no runtime effect
in consumer tests.

---

## Validation Rules

| Rule | Detail |
|------|--------|
| Authorization header | MUST be present and match `Bearer <token>` pattern in all authenticated interactions |
| Content-Type header | MUST be `application/json` for all POST/DELETE/PATCH requests with a body |
| Response `success` field | MUST be `true` in all 200 response bodies |
| Path parameters | MUST be URL-encoded (`url.PathEscape`) — verified by pact path matcher |
| Query parameters | MUST be present for `UploadInstanceStatus` (`stack`, `component`) |
