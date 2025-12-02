# Stack-Level Auth Defaults Test Fixture

This fixture tests the scenario where a default identity is configured in stack
config (e.g., `stacks/orgs/acme/_defaults.yaml`) rather than in `atmos.yaml` or
profile config.

## The Problem

Previously, Atmos could not recognize default identities set in stack configs because:
1. Auth configuration is needed BEFORE stacks are processed
2. Stack configs are only loaded AFTER authentication is configured
3. This created a chicken-and-egg problem

## The Solution

Atmos now performs a lightweight pre-scan of stack config files to find auth
identity defaults BEFORE full stack processing. This allows stack-level defaults
to be recognized.

## Test Structure

```
stack-auth-defaults/
├── atmos.yaml              # Auth config with identity (NO default)
├── stacks/
│   ├── _defaults.yaml      # Sets default: true for test-identity
│   └── test.yaml           # Test stack importing _defaults
└── components/
    └── terraform/
        └── test-component/
            └── main.tf
```

## Expected Behavior

When running `atmos describe component test-component -s test-ue2-dev`:
- The `test-identity` should be auto-detected as the default
- No interactive identity selection prompt should appear
- Authentication should proceed automatically (or fail gracefully if no credentials)

## Related

- `docs/fixes/stack-level-default-auth-identity.md` - Full issue documentation
- `pkg/config/stack_auth_scanner.go` - The scanner implementation
- `pkg/auth/manager_helpers.go` - Integration with auth manager
