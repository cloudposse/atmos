# List Instances Auth Fail-Fast

## Problem

`atmos list instances --upload` could emit the same `Initialize Identities` error once per component and keep processing after a component declared a default identity that was not configured.

For example, a missing profile for `example-root/terraform` produced repeated blocks like:

```text
Initialize Identities
Error: invalid identity config
Identity "example-root/terraform" is not configured. Did you forget to specify a profile?
```

## Root Cause

Per-component auth resolution treated resolver failures as non-fatal. When a component had `auth.identities.<name>.default: true`, the resolver attempted to create a component-specific auth manager, printed the initialization error, returned an error, and the describe-stacks processor silently fell back to the parent auth manager.

That made a fatal configuration problem look recoverable and repeated the same error for each matching component.

## Fix

Components that declare a default identity now fail fast when per-component auth resolution fails. The error is returned with component and stack context, and processing stops before template or YAML-function evaluation can continue with the wrong identity.

Auth manager initialization no longer prints these errors eagerly from the construction path. The returned error is left for the top-level command handler to format once.

## Expected Behavior

When a declared default component identity is missing or invalid:

- `atmos list instances --upload` stops on the first affected component.
- The command exits non-zero.
- The user sees a single formatted error instead of repeated `Initialize Identities` blocks.
