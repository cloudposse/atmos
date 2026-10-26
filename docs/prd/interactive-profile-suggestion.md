# PRD: Interactive Profile Suggestion When No Profile Is Active

## Context

Atmos has two separate selection mechanisms that get confused:

- **`--profile` / `ATMOS_PROFILE`** — selects an Atmos configuration profile (a directory of YAML files under `pkg/config/profiles.go`).
- **`--identity` / `ATMOS_IDENTITY`** — selects an entry under `auth.identities` in the loaded config.

If the user runs `atmos --identity foo ...` and they haven't picked a profile, and `foo` doesn't exist in the base config, Atmos errors with `ErrIdentityNotFound`. That identity might be defined in a profile the user hasn't yet selected — but historically Atmos never looked.

### Related gap: no default profile support

Before this PRD, Atmos had **no** way to declare a default profile in `atmos.yaml`. The precedence was: `--profile` flag → `ATMOS_PROFILE` env → nothing (empty). The `ProfilesConfig` schema at `pkg/schema/schema.go` only exposed `BasePath`, and `getProfilesFromFlagsOrEnv()` in `pkg/config/load.go` only checked flag + env.

This PRD bundles default-profile support alongside the interactive suggestion feature because the two share the same "no profile currently active" detection path.

## Feature 1: Default profile in atmos.yaml

The `ProfilesConfig` struct in `pkg/schema/schema.go` gains a `Default` field:

```go
type ProfilesConfig struct {
    BasePath string `yaml:"base_path,omitempty" ...`
    // Default profile loaded when neither --profile flag nor ATMOS_PROFILE is set.
    Default  string `yaml:"default,omitempty" ...`
}
```

The resolution precedence in `pkg/config/load.go:getProfilesFromFlagsOrEnv()` is:

1. `--profile` flag
2. `ATMOS_PROFILE` env var
3. **new:** `profiles.default` from the loaded atmos.yaml
4. empty (current fallthrough)

Design note: step 3 runs against the **base** atmos.yaml only — we don't recursively apply a default profile's own default. If the base declares `profiles.default: dev`, Atmos loads `dev` once as if the user had typed `--profile dev`. Anything `dev` declares for `profiles.default` is ignored (avoid cycles and surprise).

**Feature-2 interaction:** A default-from-config is treated as **implicit** — unlike `--profile` or `ATMOS_PROFILE` which are *explicit* user choices. If the user explicitly passed `--profile A` or set `ATMOS_PROFILE`, the interactive suggestion does NOT fire (we respect their choice). But if the only reason a profile loaded is `profiles.default`, the suggestion **does** still fire when an identity is missing — because the user never actively chose it.

This means `HasExplicitProfile` needs to distinguish "explicit user selection" from "loaded because of default." The simplest way: check `--profile` flag presence and `ATMOS_PROFILE` env, ignoring the loaded-config state.

## Feature 2: Interactive profile suggestion

When **all** of the following hold, prompt the user to pick a profile:

1. Identity resolution fails in `pkg/auth/manager.go:Authenticate()` (the single seam).
2. **No profile is explicitly active** — `--profile` was not passed and `ATMOS_PROFILE` is not set.
3. The terminal is interactive (`isInteractive()` in `pkg/auth/manager.go`).
4. At least one profile defines the requested identity.

When these hold:

- Show an interactive `huh.Select` listing the profiles that define the identity.
- User picks one → re-exec `atmos --profile <picked> <original args...>`.
- User cancels → normal `ErrIdentityNotFound`.

When a profile **is** already explicitly active we do nothing special — user explicitly picked their profile, we respect it. Just error.

When no profile defines the identity, we error normally.

When not interactive but a profile defines the identity, the error surfaces a hint: ``Identity `foo` is defined in profile `dev`. Run `atmos --profile dev --identity foo ...`.``

## Design

### Detect "no profile active"

`pkg/config/profiles_identity_helpers.go` exports:

```go
// HasExplicitProfile returns true if a profile was selected via --profile or ATMOS_PROFILE.
// A default-from-config does NOT count.
func HasExplicitProfile() bool
```

The check reads os.Args directly (so it works for commands with `DisableFlagParsing=true` where pflag never runs) plus `ATMOS_PROFILE` env.

### Peek a profile's identities without loading it

```go
// ProfileDefinesIdentity returns true if profileName's auth.identities map contains identityName.
// Uses a scoped Viper — does not mutate global config.
func ProfileDefinesIdentity(atmosConfig *schema.AtmosConfiguration, profileName, identityName string) (bool, error)
```

### Find all profiles defining the identity

```go
// ProfilesWithIdentity returns the names of all profiles whose auth.identities contains identityName.
func ProfilesWithIdentity(atmosConfig *schema.AtmosConfiguration, identityName string) ([]string, error)
```

Iterates known profiles and calls `ProfileDefinesIdentity` for each. Result is alphabetically sorted for deterministic output.

### The prompt

In `pkg/auth/profile_fallback.go:maybeOfferProfileFallback`, gate on the four conditions above and call `promptForProfileSelection`. One candidate → `huh.NewConfirm`; multiple → `huh.NewSelect`.

### Re-exec after selection

Cross-platform via `syscall.Exec` (Unix replaces the process; Windows uses Go's syscall shim that creates a child process and exits with its status).

New argv: `[atmos, --profile, <picked>, <original args[1:]...>]`.

Loop guard via `ATMOS_PROFILE_FALLBACK=1` env — set before exec, checked at the start of the fallback. If present, skip fallback and error normally so the user doesn't get trapped in a prompt loop.

### Error hint (non-interactive path)

When conditions 1, 2, 4 hold but the terminal is non-interactive, use the existing error builder in `errors/errors.go` to enrich `ErrIdentityNotFound`:

```
Error: identity `foo` not found
  Hint: identity `foo` is defined in profile `dev`
  Hint: Re-run with `--profile dev` to use it
```

## Critical files

| File | Change |
|---|---|
| `pkg/schema/schema.go` | Add `Default` field to `ProfilesConfig` |
| `pkg/config/load.go` | Extend `getProfilesFromFlagsOrEnv()` with step 3 (`profiles.default` from base config) |
| `pkg/config/profiles_identity_helpers.go` | Export `HasExplicitProfile`, `ProfileDefinesIdentity`, `ProfilesWithIdentity` |
| `pkg/auth/profile_fallback.go` | Gate fallback on the four conditions; call helpers; prompt; re-exec |
| `pkg/auth/manager.go` | Wire `maybeOfferProfileFallback` into `Authenticate()` before returning `ErrIdentityNotFound` |
| `pkg/auth/profile_fallback_test.go` | Tests for non-interactive path, explicit-profile gating, loop guard, re-exec argv |
| `pkg/config/profiles_identity_helpers_test.go` | Fixture tests for the new helpers |

## Feature 3: Generic fallback for all auth commands

Feature 2 fires only from `Authenticate(identityName)` — it needs a specific identity to narrow on. That misses the more common case where the user runs `atmos auth login` (no `--identity`, no `--profile`) in a repo whose auth config lives entirely in profiles. The base `atmos.yaml` has neither `auth.identities` nor `auth.providers`, so Atmos fails upfront with `ErrNoProvidersAvailable` / `ErrNoIdentitiesAvailable` / `ErrNoDefaultIdentity` — before we ever have an identity name to search on.

Feature 3 generalizes the fallback. When a caller hits one of these "no auth config in base" terminal errors, Atmos checks whether any *profile* defines auth config (identities OR providers) and, if so, offers the same profile-switch suggestion — just without narrowing to a specific identity.

### New helper — identity-agnostic profile discovery

```go
// ProfilesWithAuthConfig returns names of profiles whose atmos.yaml defines
// a non-empty auth.identities or auth.providers section. Sorted alphabetically.
func ProfilesWithAuthConfig(atmosConfig *schema.AtmosConfiguration) ([]string, error)
```

Lives alongside `ProfilesWithIdentity` in `pkg/config/profiles.go`. Uses the same scoped-Viper peek, just checks both sections instead of one specific identity.

### New auth-manager method

```go
// MaybeOfferAnyProfileFallback is the identity-agnostic sibling of the
// identity-specific fallback. Fires when the caller hit a "no identities
// / no providers / no default" terminal error, no profile is explicitly
// active, and at least one profile defines auth config.
func (m *manager) MaybeOfferAnyProfileFallback(ctx context.Context) error
```

Same four-stage gating as Feature 2: loop guard → explicit profile → candidate lookup → interactive prompt or enriched non-interactive error. Non-interactive error wraps `ErrNoIdentitiesAvailable` with explanation "No identities are defined in the currently loaded configuration." and one hint per candidate profile.

### Command hooks

Each identity-dependent auth command wraps its terminal error return with:

```go
if errors.Is(err, errUtils.ErrNoProvidersAvailable) ||
    errors.Is(err, errUtils.ErrNoIdentitiesAvailable) ||
    errors.Is(err, errUtils.ErrNoDefaultIdentity) {
    if fbErr := authManager.MaybeOfferAnyProfileFallback(ctx); fbErr != nil {
        return fbErr
    }
}
return err
```

Hooked sites: `auth login`, `auth exec`, `auth shell`, `auth env`, `auth console`, `auth whoami`. Not hooked: `auth list`, `auth validate`, `auth logout`, `auth user` (don't depend on identity resolution).

The shared helper `maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, err)` in `cmd/auth_profile_fallback.go` captures the pattern so each call site is a single line.

### Interaction with Feature 2

No conflict. Feature 2 fires inside `Authenticate(identityName)` when a *specific* identity is unresolvable. Feature 3 fires in `cmd/` when *no* identity could be chosen at all. Both share the same `ATMOS_PROFILE_FALLBACK=1` loop guard, so a re-exec from either path is suppressed on the second invocation.

## Scenarios

"Profile source" column: `flag` = `--profile` or `ATMOS_PROFILE` (explicit); `default` = `profiles.default` in base atmos.yaml (implicit); `none` = nothing.

| # | Profile source | Interactive? | Identity in a profile? | Expected |
|---|---|---|---|---|
| 1 | none | yes | no | `ErrIdentityNotFound` unchanged |
| 2 | none | yes | yes (1 profile) | Confirm → re-exec; cancel → `ErrIdentityNotFound` |
| 3 | none | yes | yes (N profiles) | Select list → pick → re-exec; cancel → `ErrIdentityNotFound` |
| 4 | none | no | yes | `ErrIdentityNotFound` with hint naming the profile(s) |
| 5 | none | no | no | `ErrIdentityNotFound` unchanged |
| 6 | flag | yes | yes (another profile) | `ErrIdentityNotFound` unchanged — respect explicit user choice |
| 7 | flag | yes | no | `ErrIdentityNotFound` unchanged |
| 8 | default | yes | yes (another profile) | Confirm/select → re-exec (default is implicit, not explicit) |
| 9 | default | no | yes | `ErrIdentityNotFound` with hint naming the profile(s) |
| 10 | n/a | n/a | n/a | `ATMOS_PROFILE_FALLBACK=1` sentinel set → skip fallback, error normally (loop guard) |
| 11 | none + `profiles.default: dev` set | n/a | n/a | `dev` profile loaded automatically (Feature 1 precedence test) |
| 12 | flag `--profile prod` + `profiles.default: dev` | n/a | n/a | `prod` loaded, `dev` ignored (Feature 1 precedence test) |

Scenarios 6–7 enforce the "if the user chose profile A, we don't load profile B just because it has the identity" rule.
Scenarios 8–9 keep the suggestion alive when the only reason a profile is loaded is the implicit default.

## Verification

1. Unit tests for scenarios 1, 4–7, 10 and the helpers (all automated).
2. Fixture profile tests for `ProfileDefinesIdentity` / `ProfilesWithIdentity` (auth.identities present/absent, case-insensitive match, sorted output).
3. **Manual end-to-end (TTY-required)**: scenarios 2, 3, 8 — run locally with two profiles defining the same identity and confirm the prompt, selection, and re-exec behavior. Automated TTY coverage is not feasible with `huh`.
4. Cross-platform build check: `GOOS=windows go build ./pkg/auth/...` passes — `syscall.Exec` has a Windows shim.
