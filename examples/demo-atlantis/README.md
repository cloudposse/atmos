---
title: Atlantis
tags: [Automation]
cast:
  file: /casts/examples/demo-atlantis/repo-config.cast
  title: atmos Atlantis repo config
related_docs:
  - label: "Atlantis commands"
    url: /cli/commands/atlantis/usage
  - label: "Atlantis configuration"
    url: /cli/configuration/integrations/atlantis
---

# Example: Demo Atlantis

Generate Atlantis configuration for PR-based Terraform automation.

Learn more about [Atlantis Integration](https://atmos.tools/cli/configuration/integrations/atlantis).

## What You'll See

- [Atlantis repo config](https://atmos.tools/cli/configuration/integrations/atlantis) generation
- [Custom commands](https://atmos.tools/cli/configuration/commands) for build automation
- Varfile generation for Atlantis projects

## Try It

```shell
cd examples/demo-atlantis

# Generate Atlantis repo configuration
atmos atlantis generate repo-config --config-template config-1 --project-template project-1

# Or use the custom command
atmos atlantis build-all
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Atlantis templates and custom commands |
| `stacks/` | Stack definitions that become Atlantis projects |
