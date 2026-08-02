---
slug: introducing-atmos-auth-list
title: 'See every identity and authentication chain you have configured'
date: 2025-10-21T12:00:00.000Z
sidebar_label: Introducing atmos auth list
authors:
  - osterman
tags:
  - feature
release: v1.196.0
---

import CastPlayer from '@site/src/components/CastPlayer'

Authentication in a real infrastructure repository stops being a single login. There is an SSO provider, a base role, an admin role per account, one identity per environment, and a different path for the security auditor than for the developer. All of that lives in YAML, and answering "which identities can reach production, and how does each one get there?" means reading the configuration end to end and holding the chains in your head. The `atmos auth list` command answers those questions from the command line instead.

<!--truncate-->

See it in action:

<CastPlayer src="/casts/examples/demo-auth/auth-list.cast" title="atmos auth identities" chrome controls scrubber />

[View the full example](/examples/demo-auth)

## The Problem

Teams routinely run several cloud providers at once — AWS, Azure, GCP, Okta — with role assumption chains that go several steps deep, such as SSO to a base role to an admin role to a specific account. Add one set of identities per environment and per team role, and the configuration outgrows what anyone keeps in their head.

Without proper tooling, it becomes difficult to answer simple questions like:

- "What authentication providers do we have configured?"
- "Which identities can I use to access production?"
- "How does this identity authenticate? Through which provider?"
- "What's the complete authentication chain for this admin role?"

## The Fix

The `atmos auth list` command reads the same authentication configuration your logins already use and prints what is in it: every provider, every identity, and the chain each identity resolves through. It is available in Atmos `v1.196.0` and later, and it needs no configuration of its own.

## How to Use It

### Multiple output formats

The default table format gives a quick overview of the key attributes:

```shell
atmos auth list
```

The tree format shows hierarchical relationships and authentication chains:

```shell
atmos auth list --format tree
```

JSON and YAML feed scripts and automation:

```shell
atmos auth list --format json | jq '.identities'
atmos auth list --format yaml > auth-config.yml
```

Graph formats produce diagrams for documentation:

```shell
atmos auth list --format graphviz > auth-chain.dot
atmos auth list --format mermaid > auth-chain.mmd
atmos auth list --format markdown > docs/auth-config.md
```

### Filtering

Filter by providers or identities to focus on what matters:

```shell
# Show only AWS SSO providers
atmos auth list --providers=aws-sso

# View specific identities
atmos auth list --identities=admin,developer

# Show all providers (no identities)
atmos auth list --providers
```

### Quick overview

```shell
$ atmos auth list

PROVIDERS
NAME      KIND       REGION      START URL                                DEFAULT
aws-sso   aws-sso    us-east-1   https://example.awsapps.com/start       ✓
okta      okta                   https://example.okta.com

IDENTITIES
NAME       KIND              VIA PROVIDER  VIA IDENTITY  DEFAULT  ALIAS
admin      aws/assume-role   aws-sso                     ✓        prod-admin
developer  aws/assume-role   aws-sso                              dev
ops        aws/assume-role   aws-sso       admin                  ops-admin
```

### Detailed tree view

```shell
$ atmos auth list --format tree

Authentication Configuration
├─ aws-sso (aws-sso) [DEFAULT]
│  ├─ Region: us-east-1
│  ├─ Start URL: https://example.awsapps.com/start
│  └─ Identities
│     ├─ admin (aws/assume-role) [DEFAULT] [ALIAS: prod-admin]
│     │  ├─ Principal
│     │  │  └─ arn: arn:aws:iam::123456789012:role/AdminRole
│     │  └─ ops (aws/assume-role) [ALIAS: ops-admin]
│     │     └─ Principal
│     │        └─ arn: arn:aws:iam::987654321098:role/OpsRole
│     └─ developer (aws/assume-role) [ALIAS: dev]
│        └─ Principal
│           └─ arn: arn:aws:iam::123456789012:role/DeveloperRole
└─ okta (okta)
    └─ URL: https://example.okta.com
```

The tree format shows the hierarchical relationship between providers and identities. Identities that authenticate through a provider appear as children under that provider's "Identities" section. Identity chains (where one identity assumes another) are shown as nested children - notice how `ops` appears as a child of `admin` since it authenticates via the `admin` identity.

### Automation

```shell
# Export to JSON for CI/CD validation
atmos auth list --format json | jq -r '.providers | keys[]'

# Generate documentation
atmos auth list --format yaml > docs/auth-config.yml

# Check if specific provider exists
atmos auth list --providers=aws-sso --format json | jq -e '.providers["aws-sso"]'
```

For the full flag reference, see the [atmos auth list command reference](/cli/commands/auth/list).

## Reading Authentication Chains

Chains show how an identity authenticates, through a provider or through another identity. The rendered chain reads left to right, from the provider that authenticates you first to the identity you end up with:

```text
aws-sso → base-role → admin-role → prod-account
```

- **Simple chain**: `aws-sso → admin`
  Direct authentication through AWS SSO

- **Multi-step chain**: `aws-sso → base-role → admin-role`
  Authenticate via SSO, assume base role, then assume admin role

- **Complex chain**: `okta → aws-dev → prod-account → admin`
  Authenticate through Okta, assume AWS dev role, switch to prod account, become admin

Chains can be arbitrarily long, which is what enterprise authentication setups tend to need.

## Where It Fits

The `atmos auth list` command sits alongside the other authentication commands, and covers the discovery step they assumed you had already done:

- **`atmos auth whoami`** - See your current authentication status
- **`atmos auth login`** - Authenticate with a provider
- **`atmos auth list`** - View all available providers and identities
- **`atmos auth validate`** - Validate authentication configuration
- **`atmos auth env`** - Export credentials as environment variables

## What's Next

This command is part of a broader authentication effort. Also on the way:

- **`atmos auth logout`** - Cleanly terminate authentication sessions and clear cached credentials
- **`atmos auth shell`** - Launch an authenticated shell session with credentials automatically configured
- **Interactive identity selection** - Improved identity selection and TTY dialogs for `atmos auth login`
- **AWS SSO improvements** - Spinners and interactive prompts during AWS SSO authentication

## Get Involved

Tell us which authentication configurations this command renders badly, and which output format you actually script against. Open an issue at [github.com/cloudposse/atmos/issues](https://github.com/cloudposse/atmos/issues).
