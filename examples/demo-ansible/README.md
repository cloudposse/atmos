# Example: Ansible Hello World

Minimal Atmos setup demonstrating how stack variables are passed to Ansible playbooks.

Learn more in the [Ansible Playbook Command](https://atmos.tools/cli/commands/ansible/playbook) docs.

## What You'll See

- Ansible [component](https://atmos.tools/components) configuration in Atmos stacks
- Variables passed from stack manifests to playbooks via `--extra-vars`
- [Catalog pattern](https://atmos.tools/howto/catalogs) for shared defaults with per-environment overrides

## Try It

```shell
cd examples/demo-ansible

# List all stacks
atmos list stacks

# Describe the hello-world component in dev
atmos describe component hello-world -s dev

# Run the playbook in dev
atmos ansible playbook hello-world -s dev

# Run the playbook in prod (different vars)
atmos ansible playbook hello-world -s prod

# Dry run to see the command without executing
atmos ansible playbook hello-world -s dev --dry-run
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Atmos configuration with Ansible component path |
| `stacks/deploy/` | Per-environment stack files (dev, prod) |
| `stacks/catalog/` | Shared component defaults |
| `components/ansible/hello-world/` | Ansible playbook and inventory |
