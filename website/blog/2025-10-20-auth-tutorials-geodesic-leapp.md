---
slug: auth-tutorials-geodesic-leapp
title: 'New Guides for Atmos Auth: Leapp Migration and Geodesic Integration'
sidebar_label: Atmos Auth Guides
authors:
  - osterman
tags:
  - documentation
date: 2025-10-20T12:00:00.000Z
release: v1.196.0
---

Changing how a team authenticates is rarely blocked by the new tool. It is blocked by the old setup, which works, which nobody has fully written down, and which somebody configured three years ago. Two new guides cover the two setups Atmos users most often have to translate from: Leapp, and Geodesic.

<!--truncate-->

## The Problem

The `atmos auth` command (introduced in v1.194.1) handles AWS IAM Identity Center authentication natively, so credentials no longer depend on a separate app. Adopting it means mapping an existing arrangement onto it — a Leapp profile list, or a container that already expects credentials to be somewhere specific. That mapping is the actual work, and it is what the guides document.

## The Fix

### [Migrating from Leapp](/tutorials/migrating-from-leapp)

If your team uses Leapp for credential management, this guide walks through the migration step by step:

- **Understanding the mapping** between Leapp concepts (providers, sessions, identities) and `atmos auth` configuration
- **Quick migration examples** showing side-by-side comparisons
- **Field-by-field reference** for converting Leapp sessions to Atmos identities
- **Troubleshooting common issues** during migration

The examples use real Leapp session configurations, so you can translate your existing setup rather than rebuild it.

### [Configuring Geodesic with Atmos Auth](/tutorials/configuring-geodesic)

For teams using [Geodesic](https://github.com/cloudposse/geodesic) as their DevOps toolbox, this guide explains how to integrate `atmos auth`:

- **Host-based authentication flow** - How authentication works on your laptop before starting Geodesic
- **Dockerfile configuration** with required environment variables
- **Makefile setup** for automatic authentication before shell start
- **Source profile configuration** for assume-role utilities
- **Complete working examples** showing all components together

The part that surprises people: authentication happens on your host machine, not inside the container. The guide covers why, and what that means for keychain access from a container.

## Why Move Authentication Into Atmos

Compared with an external credential manager:

- **Configuration as code** — authentication config lives in `atmos.yaml`, next to the infrastructure it authenticates against
- **Per-component identities** — different components can use different AWS identities
- **Nothing extra to run** — no separate credential app to install, launch, or keep in sync
- **Cross-platform** — the same setup works on Linux, macOS, and Windows
- **One approach per team** — everyone authenticates the same way, so a new teammate inherits a working setup rather than a description of one

## How to Use It

Read the guide that matches your situation — [Leapp migration](/tutorials/migrating-from-leapp) or [Geodesic configuration](/tutorials/configuring-geodesic) — alongside the [authentication user guide](/cli/commands/auth/usage) and the [command reference](/cli/commands/auth/login). Then:

```bash
# Configure providers and identities in atmos.yaml
# Then authenticate
atmos auth login

# Verify authentication
atmos auth whoami

# Use with Terraform
atmos terraform plan <component> -s <stack>
```

## Get Involved

If a guide has a gap, or your setup does not map cleanly onto either one, that is worth reporting at https://github.com/cloudposse/atmos/issues
