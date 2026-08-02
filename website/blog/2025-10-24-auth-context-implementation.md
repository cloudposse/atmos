---
slug: auth-context-implementation
title: 'Auth Context: Holding Credentials for Several Providers at Once'
date: 2025-10-24T12:00:00.000Z
authors:
  - osterman
tags:
  - core
release: v1.196.0
---

A single component deployment often needs two sets of credentials alive at the same time: AWS to reach the infrastructure, GitHub to reach the repository. When every authenticator writes its own files and exports its own environment variables, only one of them can be in effect at a time, and the last one to run wins. Atmos now carries authentication state in one place, `AuthContext`, so several providers can be active together.

<!--truncate-->

## The Problem

Credential information used to be scattered and provider-specific. Each authenticator wrote its own identity-specific files, and each set environment variables independently, which meant exactly one provider could be represented in the process environment at any moment. Nothing tracked more than one active set of provider credentials, so an authentication chain that resolved several identities had each link overwrite the credentials of the one before it. Backend operations paid for this most visibly: Terraform state access on S3 had no path to the credentials it was supposed to use.

## The Fix

The `AuthContext` struct is the single source of truth for runtime authentication credentials across providers. It is populated once, after authentication, and flows through the rest of the pipeline:

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

The pieces that make that work:

The `schema.AuthContext` type holds provider-specific fields — AWS and GitHub today, with Azure and GCP to follow the same shape. `SetAuthContext()` populates it once authentication completes, and environment variables are then derived from the context instead of each authenticator re-deriving them from its own credential logic.

The `PostAuthenticate` interface now takes a `PostAuthenticateParams` struct rather than six positional parameters, which cuts the signature to two arguments and gives new providers somewhere to put their own inputs.

Terraform backend operations accept an `authContext` parameter and use it, which is what makes `terraform.output()` and state queries resolve correctly when more than one identity is in play.

Atmos-managed AWS credentials also moved to the XDG Base Directory Specification as part of this work.

## Where AWS Credentials Live Now

Atmos-managed AWS credentials follow the same location conventions as the cache and the keyring:

- **Linux**: `~/.config/atmos/aws/<provider>/credentials`
- **macOS**: `~/.config/atmos/aws/<provider>/credentials`
- **Windows**: `%APPDATA%\atmos\aws\<provider>\credentials`

The location honors `$XDG_CONFIG_HOME` and `$ATMOS_XDG_CONFIG_HOME`, and resolves to the platform-appropriate path on its own. Keeping these files under the `atmos/` namespace also keeps them out of the user's personal AWS credentials.

Anyone with a custom `files.base_path` in their provider configuration is unaffected. Anyone on the defaults gets credentials written to the new location at their next login.

## How to Use It

Little of this is visible from the CLI, and nothing in a stack configuration changes. What changes is how auth-related code should be written from here on.

Pass `authContext` through your call chains rather than re-reading credentials at the point of use, call `SetAuthContext()` to populate it once authentication finishes, and derive whatever you need — environment variables, SDK configuration, backend settings — from the context instead of duplicating the credential logic that filled it.

Adding a provider means adding fields to `AuthContext`, not adding another parallel path for its credentials to travel. Tests get the same benefit: the context can be mocked or injected, so multi-provider scenarios no longer need real authentication to exercise.

The complete technical design is in `docs/prd/auth-context-multi-identity.md`, and the implementation landed in #1695.

## Get Involved

If you are building on the authentication pipeline and the context does not carry something you need, that is worth saying out loud — start the discussion at [GitHub Issues](https://github.com/cloudposse/atmos/issues).
