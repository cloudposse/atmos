# Dependency Processing Contracts

**Date:** 2026-09-10

## Problem

Atmos currently parses and validates component dependencies independently in direct dependent lookup, list dependency graphs, and Terraform scheduler discovery. These paths have drifted on malformed sections, custom template delimiters, optional edge metadata, and whether discovery continues after a recoverable declaration error.

## Decision

Introduce a shared dependency declaration parser in `pkg/dependency`. It accepts a component section, source stack and type, and configured delimiters. It returns modern or legacy declarations with static sentinel errors for malformed sections and invalid fields.

Each graph builder retains its own policy for parsed declarations:

- Strict execution and list graph construction reject required invalid or unavailable targets.
- Discovery records a recoverable declaration error and continues parsing later declarations from the same source component.
- Component-type-specific filtering remains in the caller.

`Graph.AddDependencyWithOptional` owns the `OptionalDependencies` map invariant and initializes it before recording either a new or duplicate edge.

## Data Flow

1. A caller resolves stacks and template delimiters.
2. The shared parser normalizes `dependencies.components` or legacy `settings.depends_on` into `schema.ComponentDependency` values.
3. The caller applies its component-kind and scope rules.
4. The caller adds graph edges or records an error according to its strict or discovery policy.

## Compatibility

- Existing `dependencies.components` and legacy `settings.depends_on` inputs remain supported.
- Public commands preserve their current strict-versus-discovery behavior.
- Existing exported graph structures remain unchanged; callers that directly populate `Graph.Nodes` become safe for optional-edge insertion.

## Validation

- A non-map `dependencies` section consistently returns `ErrInvalidDependenciesSection`.
- Custom template delimiters defer unresolved `required` selectors in bounded and unbounded dependency graphs.
- A direct `Graph.Nodes` insertion cannot panic on optional-edge insertion.
- Scheduler discovery retains valid edges after a recoverable unresolved declaration; strict graph construction still errors.

## Scope

This change covers current component dependency parsing and graph construction only. It does not redesign stack resolution, CLI selection semantics, or component type registries.
