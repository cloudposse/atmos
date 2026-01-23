# Example: Demo Atlantis

Generate Atlantis configuration for PR-based Terraform automation.

Learn more about [Atlantis Integration](https://atmos.tools/integrations/atlantis/).

## What You'll See

- [Atlantis repo config](https://atmos.tools/integrations/atlantis/repo-config/) generation
- [Custom commands](https://atmos.tools/core-concepts/custom-commands/) for build automation
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
