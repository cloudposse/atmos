---
slug: auth-context-implementation
title: "Auth Context: Centralizing Authentication State in Atmos"
authors: [atmos]
tags: [contributors, atmos-core, refactoring]
---

We've implemented a centralized authentication context system to solve credential management issues in Atmos's multi-identity authentication chain.

<!--truncate-->

## What Changed

We introduced `AuthContext` as the **single source of truth** for runtime authentication credentials. This context flows through the entire authentication pipeline and enables proper credential handling in operations like Terraform state access.

**Key changes:**
- Added `schema.AuthContext` to hold active AWS credentials (profile, region, credentials file, config file)
- Refactored `PostAuthenticate` interface to use `PostAuthenticateParams` struct (reducing parameters from 6 to 2)
- Updated Terraform backend operations to accept and use `authContext` parameter
- Created `SetAuthContext()` function to populate context after authentication
- Derived environment variables from auth context rather than duplicating credential logic

## Why This Matters

**Before:** Credential information was scattered across multiple locations:
- Identity-specific files written by each authenticator
- Environment variables set independently
- Backend operations couldn't access proper credentials for S3 state
- Multi-identity chains had no way to track active credentials

**After:** Single flow ensures consistency:
```go
Authenticate → SetupFiles → SetAuthContext → SetEnvironmentVariables
                                    ↓
                            Single source of truth
                                    ↓
                    Used by: Terraform state ops, SDK calls, spawned processes
```

## For Atmos Contributors

This is an **internal architecture improvement** with zero user-facing impact. The changes enable:

1. **Terraform state operations with proper auth** - `terraform.output()` and state queries now work correctly in multi-identity scenarios
2. **Cleaner interface design** - PostAuthenticateParams struct is more maintainable than 6 individual parameters
3. **Reduced complexity** - Extracted nested conditionals into focused helper functions
4. **Better testability** - Auth context can be mocked/injected for testing

**Related PRs:**
- #1695 - Auth context implementation
- See `docs/prd/auth-context-multi-identity.md` for complete technical design

## Get Involved

This refactoring sets the foundation for future authentication improvements. If you're working on auth-related features, ensure you:
- Pass `authContext` through your call chains
- Use `SetAuthContext()` to populate credentials
- Derive from auth context rather than duplicating credential logic

Questions? Discussion in #1695 or reach out to the core team.
