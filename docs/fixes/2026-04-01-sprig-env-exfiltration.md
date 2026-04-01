# Sprig `env`/`expandenv` Exfiltration via Stack Templates

**Date:** 2026-04-01
**Severity:** High — CWE-526 (Cleartext Storage of Sensitive Information in Environment Variables)
**Files fixed:**
- `internal/exec/template_utils.go` (cached sprig funcmap, `getSprigFuncMap`)
- `pkg/locals/resolver.go` (locals template rendering, line ~456)

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

Replace `sprig.FuncMap()` with `sprig.HermeticTxtFuncMap()` in both rendering paths:

```go
// internal/exec/template_utils.go — after
sprigFuncMapCacheOnce.Do(func() {
    sprigFuncMapCache = sprig.HermeticTxtFuncMap()
})

// pkg/locals/resolver.go — after
tmpl, err := template.New(localName).Funcs(sprig.HermeticTxtFuncMap()).Parse(strVal)
```

`HermeticTxtFuncMap` excludes `env`, `expandenv`, and other functions with external
side-effects, while keeping all pure string, math, date, list, dict, and encoding functions
that templates legitimately need.

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
