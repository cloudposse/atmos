# Fix: Validate merged authentication configuration in a selected stack

**Date:** 2026-10-22

## Summary

`atmos auth validate --stack <stack> [--component <component>]` checks each selected concrete
component's merged authentication configuration. Unscoped validation remains strict.

## Context

Issue #3480 reports an identity declared in `atmos.yaml` whose principal is supplied by a stack.
Component execution supports this inheritance, but auth validation previously checked only the
global configuration and offered no stack selector. The original binary reproduced a missing
principal error and rejected `--stack` as an unknown flag.

## Changes

- Add `--stack/-s` and `--component/-c`, preserving global config/profile/path selectors.
- Resolve normal stack imports and inheritance, then reuse the shared component auth merge and
  strict provider, identity, and chain validators independently for each concrete component.
- Include all component types, exclude abstract components, and fail on missing selections.
- Disable template rendering and component YAML-function evaluation; never authenticate during
  validation. Report unresolved dynamic auth values as incomplete validation without printing them.
- Document global versus scoped validation and dynamic-expression limitations in CLI help,
  command documentation, and the auth skill.

## Validation

- Baseline binary: `auth validate` failed with exit 2 for the reported split identity, and
  `auth validate --stack dev --component demo` failed with exit 1 (unknown flag).
- Rebuilt binary: both `auth validate --stack dev` and `auth validate --stack dev --component demo`
  passed the reported fixture; global validation continued to fail with exit 2.
- Added regression tests failed against the original implementation before adding scoped flags.
- `go test ./cmd/auth ./pkg/auth/validation -count=1` passed after implementation.
- Tests cover imported and component principals, multiple stacks and component types, abstract and
  empty selections, invalid providers/identity chains, custom delimiters and unresolved functions,
  no command/credential-file side effects, shared configuration immutability, and global selectors.

## Follow-ups

None. Dynamic stack expressions intentionally remain unevaluated; their auth values must be static
for scoped validation to complete. Dynamic import paths and manifests requiring template rendering
are documented as unsupported by this static check.
