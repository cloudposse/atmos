# Starlark YAML Values

## Status

Implemented. This PRD defines `!starlark` for computed values in
stack manifests. It complements executable Atmos Automation Language scripts
and the exploratory [native stack definitions](starlark-stack-definitions.md)
proposal.

## Purpose

Let stack authors express computed configuration with ordinary functions,
conditionals, loops, and structured values while retaining YAML for the
surrounding declaration. Authors use the same Starlark syntax as their Atmos
automation and can read the consuming component's effective configuration.

```yaml
vars:
  resource_tags: !starlark |
    return {
        "Environment": ctx.vars["stage"],
        "Region": ctx.vars["region"],
        "Owner": ctx.metadata.get("owner", "platform"),
    }
```

## User Stories

- As a stack author, I can derive tags, lists, and settings from the component's
  merged variables and metadata without converting structured data to text.
- As a catalog maintainer, I can inherit one expression into several components
  and have each instance evaluate using its own overrides.
- As a maintainer, I can read another computed value without depending on YAML
  key order and receive a source-aware diagnostic for cyclic dependencies.
- As an operator, I can inspect computed configuration without the Starlark host
  launching processes, prompting, installing tools, or performing deployments.

## Syntax and Return Contract

The tag accepts a scalar function body. The preferred spelling is a literal
block scalar, `!starlark |`. Single-line bodies such as `!starlark return 3` are
also supported. The host supplies an implicit function scope; `return`, early
returns, local assignments, and nested helper functions use normal Starlark
semantics. The AST is wrapped rather than indenting and rewriting source, so
errors retain original source lines.

The expression remains a scalar through inheritance: a higher-precedence
expression or concrete value replaces the entire field. The returned value
replaces the tagged configuration value and does not participate in deferred
merging with lower-precedence maps. Supported values
are null (`None`), booleans, strings, finite numbers, lists, and dictionaries with
string keys, recursively. Empty lists and dictionaries retain their types.
Integer precision is preserved within signed/unsigned 64-bit configuration
values; larger integers and unsupported types fail explicitly. Cyclic output
collections fail. Fallthrough has normal function semantics and returns null.
The `output` global remains the result convention of executable scripts and
script steps; this tag uses the function's return value.

## Context

Context is read-only and belongs to the consuming component after configuration
imports, inheritance, overrides, and the ordinary template pass. Source code
inside the tag bypasses Go-template interpretation.

| Attribute | Meaning |
| --- | --- |
| `ctx.vars` | Effective component variables. |
| `ctx.metadata` | Effective component metadata, subject to existing metadata inheritance settings. |
| `ctx.settings` | Effective component settings. |
| `ctx.env` | Declared component environment; not the ambient process environment. |
| `ctx.locals` | Locals available in the component's resolved configuration. |
| `ctx.stack` | Current stack name. |
| `ctx.component` | Current component name, where the host supplies one. |
| `ctx.component_type` | Component type, where the host supplies one. |

Sections absent from the component are empty mappings. Mapping access supports
indexing, membership, iteration, length, and `get`, `keys`, `values`, and `items`.
Keys are iterated in sorted order. Context mutations fail. The evaluator copies
returned values so a component cannot mutate another component or a cached
configuration through shared interpreter objects.

## Evaluation and Dependencies

- Register `starlark` as a post-merge YAML function and resolve the interpreter
  through the existing interpreter registry's optional typed-value capability.
- Complete existing YAML functions and their typed merges before evaluating the
  recorded Starlark source fields. Strings returned by other functions remain
  data, even when they begin with `!starlark`.
- Read context fields lazily. Resolve another computed field when it is read,
  cache the result within this component invocation, and detect re-entry using
  the full field path. Separate invocations and components do not share results.
- Existing YAML functions read through context use their existing handlers and
  skip policies. Reading such a field retains that function's effects and
  credential requirements; the restricted Starlark host does not change them.
- Resolve dependencies even when they appear after their consumer in YAML.
- Error on cycles with the field chain and preserve source file/line diagnostics
  through imports, inheritance, and template serialization.
- Respect function skip flags. Resolving functions is separate from loading and
  structural merging; syntax inside the body never becomes a Go template.
- Returned strings are data and are not recursively interpreted as new tags.

The existing pipeline evaluates Go templates before YAML functions. Starlark can
consume template-rendered values. Go-template expressions that need the result
of a `!starlark` value in the same pass are outside this initial contract; use
Starlark for that dependency. Import paths, stack discovery, inheritance selectors,
and other fields consumed before component evaluation are also outside this
post-merge feature.

`metadata.tags` and `metadata.labels` reject `!starlark` values. These fields
drive component selection before full evaluation, while Starlark context reads
can resolve dependencies that require authentication or execute commands.

## Host Capabilities

The evaluator uses the Atmos Starlark dialect and standard pure built-ins,
including `sum`, `round`, and JSON conversion. It supplies no command, step,
filesystem, toolchain, identity, or interactive host modules. Local module loading
is outside this first version; reusable helper functions may be declared within
the body. The interpreter enforces a computation-step budget and honors context
cancellation. These are execution controls, not hard process-memory isolation.

The tag is supported in stack manifest component values and inherited data
sections. `atmos.yaml` and scaffold manifests reject it with their existing
context-specific supported-tag errors; they do not supply a merged component
context. Executable scripts in those hosts continue using script steps.

## Architecture

- `pkg/function`: registry entry and host-injected evaluation callback.
- `pkg/function/starlarksource`: source provenance and template protection,
  independent of the runtime and stack processor.
- `pkg/script`: optional typed configuration evaluator and lazy read-only map
  interface, with no Starlark-specific values across the boundary.
- `pkg/script/starlark`: implicit function compilation, pure globals, budget,
  context adapters, and native value conversion.
- `internal/exec`: component-scoped dependency resolution and integration with
  the existing YAML-function pipeline.

Existing executable interpreter APIs retain their current behavior. No full
native stack-definition API, new language runtime, or schema configuration option
is introduced. Editor custom-tag declarations and tag allowlists must recognize
the new scalar tag; resolved values follow the existing manifest schema.

## Acceptance and Validation

1. The resource-tags example returns a native map with values from vars and metadata.
2. Early returns, nested helpers, nulls, empty collections, and large exact integers work.
3. Overrides and inheritance evaluate independently per consuming component.
4. Computed dependencies work in either key order; direct and indirect cycles fail.
5. Existing YAML-function dependencies and skip flags retain their behavior.
6. Context mutation, unavailable host APIs, invalid return types, and infinite loops fail.
7. Source braces survive Go templates; errors identify the original YAML file and line.
8. Description and execution paths resolve the same values; shared caches remain unchanged.
9. Runtime, source-protection, and pipeline tests pass, including concurrent invocations.
10. Documentation examples run with valid and invalid inputs; production docs build passes.
11. The feature is delivered as a signed PR atop the existing stack, below 150 changed files.
