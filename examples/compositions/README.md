---
title: Compositions
tags: [Stacks]
cast:
  file: /casts/examples/compositions/validate.cast
  title: atmos composition validation
---

# Compositions

A **composition** defines a reusable slice of a stack across environments. The
top-level `compositions:` section declares every service that can belong to the
slice, while each stack fulfills the subset it needs. Local development can run
`frontend` and `api` while pointing at an external `database`; dev can use the
same composition and provide all three.

Components join the slice with the first-class `composition:` field. Membership
is a **closed contract** (a component may only claim a service the composition
declares), but fulfillment is **open** (a declared service with no component in a
given stack is allowed).

This example declares a `storefront` composition with three services —
`frontend`, `api`, `database` — and shows the **same composition adapting per
stack**:

| Stack   | frontend | api | database |
|---------|----------|-----|----------|
| `local` | ✅        | ✅   | — (external) |
| `dev`   | ✅        | ✅   | ✅        |

## Layout

```text
atmos.yaml                      # compositions.storefront.services
stacks/deploy/local.yaml        # fulfills frontend + api
stacks/deploy/dev.yaml          # fulfills frontend + api + database
```

## Validate membership

`atmos composition validate <name> -s <stack>` reports which declared services are
fulfilled by components in a stack and which are not provided there:

```shell
# local provides 2 of 3 services — database is declared but not provided here.
atmos composition validate storefront -s local
#   Composition: storefront
#   ✓ Fulfilled: api, frontend
#   ▶ Not provided here: database

# dev provides all three.
atmos composition validate storefront -s dev
#   Composition: storefront
#   ✓ Fulfilled: api, database, frontend
```

## Membership is a closed contract

Declaring `composition: storefront` on a component whose name is **not** in
`compositions.storefront.services` is invalid. For example:

```yaml
components:
  container:
    cache:                       # "cache" is NOT a declared storefront service
      composition: storefront
      image: redis:alpine
```

- Operating the component is a **hard error**:

  ```shell
  atmos container up cache -s local
  #   Error: component claims membership in a service not declared by the composition
  ```

- `atmos composition validate` flags it as an **unknown member**:

  ```text
  ⚠ Unknown members (not declared in services): cache
  ```

Add the service to `compositions.storefront.services` first to allow it.

## Members are ordinary components

The members here are [container components](../container-component), but a
composition can group any component kinds. Operate each member with its own
component commands (e.g. `atmos container up frontend -s local`), and `atmos container
list` shows their running state.
