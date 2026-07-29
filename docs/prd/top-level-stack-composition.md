# PRD: Top-Level Stack Discovery with Parent-Scoped Components

## Status

Implemented.

## Overview

Atmos may discover more than one top-level stack manifest with the same
canonical stack identity. Those manifests expose one logical stack for stack
listing, describe output, and component execution without becoming one
deep-merged configuration file.

## Goals

1. Allow a logical stack to be split into parent manifests by component.
2. Keep every concrete component instance owned by one parent manifest.
3. Permit the same imported component to appear through multiple parents when
    its resolved configuration is identical.
4. Keep each parent's inheritance graph self-contained: inherited bases must be
    defined inline or available through that parent's import graph.
5. Keep resolution deterministic and preserve source and provenance output.

## Non-Goals

- Deep-merging peer parent manifests.
- Assembling one component definition from multiple peer parents.
- Allowing conflicting definitions of the same component instance.
- Resolving inheritance from another parent solely because it shares the same
  logical stack identity.

## Terminology

- **Parent manifest**: A top-level stack manifest discovered by Atmos after
  imports are resolved.
- **Logical stack**: Parent manifests that resolve to the same canonical stack
  identity using `name`, `name_template`, `name_pattern`, or filename.
- **Component owner**: The parent manifest that defines a component after its
  own imports are resolved.
- **Canonical duplicate source**: The lexically first parent manifest supplying
  an equivalent duplicate component.

## Behavior

### Parent-Scoped Resolution

Each parent manifest processes imports, globals, locals, component-type
configuration, and components independently. Atmos aggregates the resulting
distinct component names under one logical stack, but does not merge peer
parents' `vars`, `settings`, `env`, locals, or component configuration.

For example, `dns-primary` can be owned by `parents/01-network.yaml` while
`chatops` is owned by `parents/02-platform.yaml`. Each receives only its own
parent's global scope.

### Equivalent Imported Duplicates

The same component can appear in multiple parent manifests when both parents
import the same catalog configuration. Atmos compares the resolved component
configuration after excluding source and provenance-only fields.

- Equal configurations are valid. The lexically first parent is the canonical
  `StackFile` for execution, describe output, sources, and provenance.
- Different configurations are ambiguous and invalid. `validate stacks` and
  component resolution report every conflicting parent manifest.

### Explicit Inheritance Dependencies

Each parent resolves `metadata.inherits` from the components it defines inline
or receives through its normal import graph. Grouping parent manifests under one
logical stack does not expand inheritance visibility.

When multiple parents need the same base, place it in a shared catalog and
explicitly import that catalog from each parent that depends on it. A base
defined only by another parent remains unresolved even when both parents have
the same canonical stack identity.

## Determinism and Compatibility

Canonical stack identity continues to follow the precedence in
[Stack Name Identity](stack-name-identity.md). Parent paths are sorted only to
choose the canonical source for equivalent duplicates; ordering does not merge
configuration values between parents.

Single-manifest stacks and normal import behavior are unchanged.

## Success Criteria

1. `describe stacks` lists one logical stack containing distinct components
    from each parent.
2. `describe component` and execution select the owning parent for unique
    components and the lexical canonical source for equivalent duplicates.
3. `metadata.inherits` resolves bases defined inline or explicitly imported by
    the owning parent.
4. Parent-specific scope does not leak between peer-owned components.
5. Differing duplicate instances and peer-only inheritance fail with actionable
    diagnostics.
