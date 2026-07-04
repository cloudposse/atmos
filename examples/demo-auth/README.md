---
title: Authentication
tags: [Stacks]
cast:
  file: /casts/examples/demo-auth/auth-list.cast
  title: atmos auth identities
---

# Demo: Auth

This demo showcases how to structure an Atmos project with an `auth` section in `atmos.yaml`. Please see [the Atmos docs](https://atmos.tools/cli/commands/auth/usage) for more information.

It intentionally only validates stack manifests (no Terraform applies) to avoid requiring real cloud credentials in CI.
