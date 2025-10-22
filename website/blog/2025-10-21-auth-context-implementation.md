---
slug: auth-context-implementation
title: "Auth Context: Centralizing Authentication State in Atmos"
authors: [atmos]
tags: [contributors, atmos-core, refactoring]
---

We've implemented a centralized authentication context system to enable **concurrent multi-provider identities** - allowing Atmos to manage AWS, GitHub, and other cloud provider credentials simultaneously in a single operation.

<!--truncate-->

## What Changed

We introduced `AuthContext` as the **single source of truth** for runtime authentication credentials across multiple cloud providers. This context flows through the entire authentication pipeline and enables concurrent identity management (e.g., AWS and GitHub at the same time) plus proper credential handling in operations like Terraform state access.

**Key changes:**
- Added `schema.AuthContext` with provider-specific fields (AWS, GitHub, future: Azure/GCP)
- Refactored `PostAuthenticate` interface to use `PostAuthenticateParams` struct (reducing parameters from 6 to 2)
- Updated Terraform backend operations to accept and use `authContext` parameter
- Created `SetAuthContext()` function to populate context after authentication
- Derived environment variables from auth context rather than duplicating credential logic

## Why This Matters

**The core problem:** Atmos needs to support **multiple cloud providers simultaneously** in a single component deployment. For example, a component might need AWS credentials for infrastructure AND GitHub credentials for repository management - both active at the same time.

**Before:** Credential information was scattered and provider-specific:
- Identity-specific files written by each authenticator
- Environment variables set independently (only one provider at a time)
- Backend operations couldn't access proper credentials for S3 state
- No way to track multiple active provider credentials concurrently
- Multi-identity chains overwrote each other's credentials

**After:** Unified context supports concurrent providers:
```go
Authenticate → SetupFiles → SetAuthContext → SetEnvironmentVariables
                                    ↓
                AuthContext {
                    AWS: { profile, region, creds... }
                    GitHub: { token, org... }
                    // Future: Azure, GCP, etc.
                }
                                    ↓
                Used by: Terraform state ops, SDK calls, spawned processes
```

## For Atmos Contributors

This is an **internal architecture improvement** with zero user-facing impact. The changes enable:

1. **Concurrent multi-provider support** - Components can use AWS + GitHub + other providers simultaneously without credentials conflicting
2. **Terraform state operations with proper auth** - `terraform.output()` and state queries now work correctly in multi-identity scenarios
3. **Cleaner interface design** - PostAuthenticateParams struct is more maintainable than 6 individual parameters
4. **Extensibility** - Adding new providers (Azure, GCP) just means adding fields to AuthContext
5. **Better testability** - Auth context can be mocked/injected for testing

**Related PRs:**
- #1695 - Auth context implementation
- See `docs/prd/auth-context-multi-identity.md` for complete technical design

## Get Involved

This refactoring sets the foundation for future authentication improvements. If you're working on auth-related features, ensure you:
- Pass `authContext` through your call chains
- Use `SetAuthContext()` to populate credentials
- Derive from auth context rather than duplicating credential logic

Questions? Discussion in #1695 or reach out to the core team.
