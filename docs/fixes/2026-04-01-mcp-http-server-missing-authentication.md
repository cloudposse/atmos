# MCP HTTP Server Has No Authentication (CWE-306)

**Date:** 2026-04-01
**Severity:** High — any process on the same network can connect to the `/sse` and `/message` endpoints
and issue MCP tool calls that read or write files in `components/` and `stacks/` directories
**Affected:** `atmos mcp start --transport http` (all versions that include the HTTP transport)
**Vulnerability class:** CWE-306 — Missing Authentication for Critical Function

---

## Symptom

```bash
# Start MCP server bound to all interfaces — no authentication required by clients
atmos mcp start --transport http --host 0.0.0.0 --port 8080
```

Any process on the network (or on the same host) can connect to the server and call any registered
MCP tool without supplying credentials:

```bash
# Unauthenticated — should be rejected, but was not
curl -N http://target:8080/sse
curl -X POST http://target:8080/message -d '{"jsonrpc":"2.0","method":"tools/call",...}'
```

Available tools include `write_component_file` and `write_stack_file`, which can write arbitrary
content to the repository's component and stack directories.

A second issue compounded the risk: `initializeAIComponents` unconditionally set
`permConfig.YOLOMode = true`, bypassing all configured permission checks regardless of
`ai.tools.yolo_mode` in `atmos.yaml`. This meant even operators who intentionally left YOLO mode
disabled (the default) were silently running with no permission enforcement.

---

## Root Cause

### 1. No authentication middleware on the HTTP server

`startHTTPServer` attached the MCP SSE handler directly to a plain `http.Server` with no
authentication layer:

```go
// before fix — cmd/mcp/server/start.go
handler := mcpsdk.NewSSEHandler(func(req *http.Request) *mcpsdk.Server {
    return server.SDK()
}, nil)

httpServer := &http.Server{
    Addr:    addr,
    Handler: handler,   // ← no auth middleware
    ...
}
```

### 2. Unconditional YOLO mode override

`initializeAIComponents` overrode the permission config with `YOLOMode = true` regardless of
what `atmos.yaml` specified:

```go
// before fix — cmd/mcp/server/start.go
permConfig := &permission.Config{
    Mode:     getPermissionMode(atmosConfig),
    YOLOMode: atmosConfig.AI.Tools.YOLOMode,
    ...
}
// Use YOLO mode for MCP to avoid blocking on prompts (client handles permissions).
permConfig.YOLOMode = true   // ← unconditional override
```

The comment acknowledged the bypass but offered no mechanism for operators to opt out.

---

## Affected Code Paths

| File                          | Location                   | Issue                                          |
|-------------------------------|----------------------------|------------------------------------------------|
| `cmd/mcp/server/start.go`     | `startHTTPServer`          | No authentication middleware on HTTP handler   |
| `cmd/mcp/server/start.go`     | `initializeAIComponents`   | `permConfig.YOLOMode = true` unconditional      |

---

## Fix

### 1. Bearer-token middleware (`bearerTokenMiddleware`)

A new `bearerTokenMiddleware` function wraps the MCP SSE handler and validates
`Authorization: Bearer <token>` on every incoming request. It uses
`crypto/subtle.ConstantTimeCompare` to prevent timing-based token enumeration:

```go
func bearerTokenMiddleware(apiKey string, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")
        const prefix = "Bearer "
        if !strings.HasPrefix(authHeader, prefix) {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        provided := authHeader[len(prefix):]
        if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

The middleware is applied to the handler when an API key is configured:

```go
if apiKey != "" {
    handler = bearerTokenMiddleware(apiKey, handler)
}
```

### 2. `--api-key` flag and `ATMOS_MCP_API_KEY` environment variable

A new `--api-key` flag was added to `atmos mcp start`. When the flag is not set, the value of
`ATMOS_MCP_API_KEY` is used as a fallback:

```bash
# Explicit flag
atmos mcp start --transport http --host 0.0.0.0 --port 8080 --api-key "my-secret-token"

# Environment variable
ATMOS_MCP_API_KEY=my-secret-token atmos mcp start --transport http --host 0.0.0.0 --port 8080
```

### 3. Non-loopback HTTP binding requires an API key

`getTransportConfig` now rejects HTTP transport configurations that bind to a non-loopback
address without an API key, returning a new sentinel error `ErrMCPHTTPAuthRequired`:

```go
if transportType == transportHTTP && !isLoopbackHost(host) && apiKey == "" {
    return nil, fmt.Errorf("%w: --api-key (or ATMOS_MCP_API_KEY) is required when --host is a non-loopback address",
        errUtils.ErrMCPHTTPAuthRequired)
}
```

Recognised loopback addresses: `localhost`, `127.x.x.x` (any IPv4 loopback), `::1`.

Loopback bindings without an API key remain allowed (for local development with desktop AI
clients) but display a warning at startup.

### 4. Removed unconditional YOLO mode override

The `permConfig.YOLOMode = true` line was removed. Permission mode is now read exclusively from
`ai.tools.yolo_mode` in `atmos.yaml` via `getPermissionMode`:

```go
// after fix
permConfig := &permission.Config{
    Mode:     getPermissionMode(atmosConfig),
    YOLOMode: atmosConfig.AI.Tools.YOLOMode,
    ...
}
// no override — respects configuration
```

---

## Operator Usage After Fix

```bash
# Local development (loopback — no key required, warning shown)
atmos mcp start --transport http --host 127.0.0.1 --port 8080

# Remote/shared access (requires API key)
atmos mcp start --transport http --host 0.0.0.0 --port 8080 --api-key "$(openssl rand -hex 32)"

# Client must supply the token
curl -N -H "Authorization: Bearer <token>" http://server:8080/sse
```

---

## Files Changed

| File                          | Change                                                                                     |
|-------------------------------|--------------------------------------------------------------------------------------------|
| `cmd/mcp/server/start.go`     | Added `bearerTokenMiddleware`, `isLoopbackHost`; `--api-key` flag; non-loopback enforcement; removed YOLO override; updated `logServerInfo` to report auth status |
| `cmd/mcp/server/start_test.go`| Updated existing tests for new signatures; added tests for `isLoopbackHost`, `bearerTokenMiddleware`, env-var fallback, non-loopback enforcement, YOLO-mode removal |
| `errors/errors.go`            | Added `ErrMCPHTTPAuthRequired` sentinel error                                              |

---

## Backward Compatibility

- `stdio` transport is unaffected — it communicates over stdin/stdout and has no network exposure.
- `http` transport on loopback (`localhost`, `127.0.0.1`, `::1`) continues to work without a key.
- `http` transport on non-loopback addresses now **requires** `--api-key` or `ATMOS_MCP_API_KEY`.
- YOLO mode previously enabled unconditionally for MCP is now off by default; enable with
  `ai.tools.yolo_mode: true` in `atmos.yaml`.

---

## Verification

1. Non-loopback HTTP without an API key returns an error at startup (no server starts).
2. Requests to `/sse` and `/message` without a valid `Authorization: Bearer` header receive
   HTTP 401 Unauthorized.
3. Token comparison uses `crypto/subtle.ConstantTimeCompare` (timing-safe).
4. All existing tests pass; 10+ new tests cover the new authentication paths.
