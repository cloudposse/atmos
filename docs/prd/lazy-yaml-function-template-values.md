# PRD: Lazy YAML-function values in template lookups (auto-deref)

## Status

Proposed (follow-up to the `!git.*` repository functions + `atmos.Resolve` work).

## Problem

Atmos evaluates Go templates **before** it evaluates `!`-prefixed YAML functions
(`internal/exec/describe_stacks_component_processor.go`: `ProcessTmplWithDatasources`
runs, then `ProcessCustomYamlTags`). A custom-tagged value is stored in the merged
section map as a plain string — `repo: !git.repository` becomes the string
`"!git.repository"` in `info.ComponentSection`.

Consequently, the natural intermediate-variable pattern does **not** work:

```yaml
settings:
  context:
    repo: !git.repository
terraform:
  workspace_key_prefix: "{{ .settings.context.repo }}/..."   # renders "!git.repository/..."
```

At template-eval time `.settings.context.repo` is still the literal string
`"!git.repository"`, so the template renders the unresolved tag. Today the supported
workarounds are:

1. `!exec` shell concatenation, or
2. the [`atmos.Resolve`](/functions/template/atmos.Resolve) template function —
   `{{ atmos.Resolve .settings.context.repo }}` — which resolves the string explicitly.

Users have asked for the **bare dereference** to "just work" — i.e.
`{{ .settings.context.repo }}` should auto-evaluate the YAML function.

## Goal

Allow a bare template dereference of a YAML-function-tagged value to resolve
automatically when the value is **printed** in a template, without an explicit
`atmos.Resolve` call.

## Proposed approach: lazy `fmt.Stringer` wrapper

Go's `text/template` provides no hook to intercept plain map/field indexing
(`{{ .a.b.c }}` reflects into the map and returns the stored value). The one usable
lever is that when a template **prints** a value, it honors `fmt.Stringer`.

Approach: before rendering a component section's templates, wrap any string value that
begins with a known Atmos YAML-function tag (per `u.AtmosYamlTags`) in a small lazy
type that implements `String()` by running the YAML-function processor on demand:

```go
type lazyYAMLValue struct {
    raw   string
    // closure/deps needed to resolve (atmosConfig, stack, stackInfo)
}

func (l lazyYAMLValue) String() string { /* processCustomTags(l.raw, ...) */ }
```

Then `{{ .settings.context.repo }}` prints the resolved value, while the original
`!git.repository` tag in the section still resolves independently in the later eager
YAML-function pass.

### Why this is decoupled (verified)

`processComponentSectionTemplates` serializes the section to a YAML string for
rendering and re-unmarshals the **rendered output** into a fresh map; the eager
YAML-function pass operates on that fresh map, not on the template-lookup data. So
wrapping values only in the template-lookup map (`info.ComponentSection`) does not
corrupt the real YAML-function resolution.

## Caveats / open questions (must be addressed before implementing)

1. **Print-context only.** `Stringer` fires for `{{ .x }}`, `printf "%s"`, and string
   concatenation, but NOT for `{{ if .x }}`, `range`, or comparisons — those see the
   wrapper struct. Need to decide behavior (e.g. also implement comparison-friendly
   interfaces, or document the limitation).
2. **`mapstructure.Decode` interplay.** The settings section is decoded into
   `schema.Settings` (`describe_stacks_component_processor.go:672`). A non-string
   wrapper could break that decode — wrapping must be scoped to the render data only,
   applied after the settings struct is extracted.
3. **Idempotency / double-resolution.** The same logical value resolves in both the
   template phase (lazy) and the eager pass. Fine for pure/read-only functions
   (`!git.*`, `!env`); needs care for side-effecting/expensive ones
   (`!exec`, `!terraform.output`). Consider restricting auto-deref to a safe subset.
4. **Cycle detection.** Lazy resolution during templating must reuse the existing
   `ResolutionContext` guard used by `!terraform.output` / `!terraform.state`.
5. **Performance.** Wrapping requires an extra walk of the section before templating;
   measure overhead on large stacks.

## Alternatives considered

- **`atmos.Resolve` (shipped).** Explicit, contained, low-risk; requires the user to
  call the function. This PRD is about removing that requirement for the bare-deref
  ergonomic.
- **Reordering passes (YAML functions before templates).** Rejected — broad,
  high-risk change to established evaluation semantics.

## Tracking

A GitHub issue should be opened to track this follow-up **only with explicit
maintainer approval** (do not auto-open). Link the issue number here and from the
related changelog posts once created.
