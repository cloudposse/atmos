---
name: atmos-ansible
description: "Ansible orchestration: playbook execution, variable passing, inventory management, stack-based configuration for configuration management"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Ansible Orchestration

Atmos wraps the Ansible CLI to provide stack-aware orchestration of configuration management operations. Instead of
manually managing variables, inventory, and playbook paths for each Ansible component, Atmos resolves the full
configuration from stack manifests and handles all of these concerns automatically.

> **Note:** `atmos ansible playbook` is designed for interactive operator sessions where a human is present to
> monitor output, respond to prompts, and approve changes. It is not suitable for headless CI/CD automation where
> no interactive terminal is available. For automated pipelines, use a dedicated Ansible CI runner or invoke
> `ansible-playbook` directly from a CI step with pre-exported credentials.

## How Atmos Orchestrates Ansible

When you run `atmos ansible playbook`, Atmos performs the following sequence:

1. **Resolves stack configuration** -- Reads and deep-merges all stack manifests to produce the fully resolved
   configuration for the target component in the target stack.
2. **Generates a variables file** -- Writes a YAML file containing all `vars` defined for the component in the
   stack, following the naming convention `<context>-<component>.ansible.vars.yaml`.
3. **Resolves the playbook** -- Determines the playbook to run from `--playbook` flag or
   `settings.ansible.playbook` in the stack manifest.
4. **Resolves the inventory** -- Determines the inventory source from `--inventory` flag or
   `settings.ansible.inventory` in the stack manifest.
5. **Sets environment variables** -- Applies all `env` settings from the stack manifest.
6. **Executes `ansible-playbook`** -- Runs the playbook in the component directory, passing the generated
   variables file via `--extra-vars @<varfile>` and any additional native flags.
7. **Cleans up** -- Removes the generated variables file after execution completes.

This means a single command like `atmos ansible playbook webserver -s prod` replaces what would normally require
finding the right playbook, constructing extra-vars, specifying inventory, and managing environment variables
manually.

## Core Commands

### playbook

Run an Ansible playbook for a component in a stack.

```shell
atmos ansible playbook <component> -s <stack> [flags] [-- ansible-options]
```

```shell
# Basic playbook execution
atmos ansible playbook webserver -s prod

# Specify playbook explicitly (overrides stack manifest)
atmos ansible playbook webserver -s prod --playbook deploy.yml

# Specify both playbook and inventory
atmos ansible playbook webserver -s prod -p site.yml -i inventory/production

# Dry run (shows command without executing)
atmos ansible playbook webserver -s prod --dry-run

# Pass native ansible-playbook flags after --
atmos ansible playbook webserver -s prod -- --check
atmos ansible playbook webserver -s prod -- -vvv
atmos ansible playbook webserver -s prod -- --limit "web01,web02"
atmos ansible playbook webserver -s prod -- --tags "deploy,config"
atmos ansible playbook webserver -s prod -- --skip-tags "slow"
atmos ansible playbook webserver -s prod -- --extra-vars "version=1.2.3"
```

### version

Display the installed Ansible version and configuration information.

```shell
atmos ansible version
```

This runs `ansible --version` and shows the Ansible version, configuration file location, module search path,
Python version, and other details.

## Stack Configuration

### Component Schema

Ansible components live under `components.ansible` in stack manifests:

```yaml
components:
  ansible:
    <component_name>:
      vars: {}              # Variables passed via --extra-vars
      env: {}               # Environment variables during execution
      settings:
        ansible:
          playbook: <file>  # Playbook file to run
          inventory: <src>  # Inventory source (file, directory, or script)
      metadata: {}          # Component behavior and inheritance
      command: ansible      # Override ansible binary
      hooks: {}             # Lifecycle event handlers
```

### Minimal Example

```yaml
components:
  ansible:
    hello-world:
      vars:
        app_name: my-app
        app_version: "1.0.0"
        app_port: 8080
      settings:
        ansible:
          playbook: site.yml
          inventory: inventory.ini
```

### Complete Stack Example

```yaml
import:
  - catalog/ansible/_defaults
  - orgs/acme/plat/prod/_defaults

vars:
  region: us-east-1
  stage: prod

ansible:
  vars:
    managed_by: Atmos
  env:
    # Security note: Setting ANSIBLE_HOST_KEY_CHECKING to "false" disables SSH host key verification,
    # which exposes connections to man-in-the-middle attacks. Only use this in controlled environments
    # (e.g., ephemeral CI environments with known-clean network paths). For production environments,
    # maintain and distribute a known_hosts file or use the ansible_ssh_extra_args approach with
    # StrictHostKeyChecking=accept-new to accept keys on first connection only.
    ANSIBLE_HOST_KEY_CHECKING: "false"

components:
  ansible:
    webserver:
      vars:
        app_name: myapp
        app_port: 8080
        app_version: "2.0.0"
      settings:
        ansible:
          playbook: site.yml
          inventory: inventory/production

    database:
      metadata:
        component: database
      vars:
        db_name: acme-prod
        db_port: 5432
      settings:
        ansible:
          playbook: deploy.yml
          inventory: inventory/production
        depends_on:
          - component: webserver
```

### Component-Type Defaults

Define defaults that apply to all Ansible components in a stack:

```yaml
# These apply to all Ansible components
ansible:
  vars:
    managed_by: Atmos
  env:
    # Security note: ANSIBLE_HOST_KEY_CHECKING: "false" disables SSH host key verification and should
    # only be used in controlled environments. For production, use a known_hosts file instead.
    ANSIBLE_HOST_KEY_CHECKING: "false"
    ANSIBLE_FORCE_COLOR: "true"

# Individual components inherit and can override
components:
  ansible:
    webserver:
      vars:
        app_port: 8080
```

## Variable Handling

Atmos generates a YAML file from the `vars` section and passes it to `ansible-playbook` using
`--extra-vars @<filename>`. All stack variables become available directly in playbooks.

The generated file follows the naming convention: `<context>-<component>.ansible.vars.yaml`

For example, for component `webserver` with context prefix `acme-plat-prod-us-east-1`, the file is:
`acme-plat-prod-us-east-1-webserver.ansible.vars.yaml`

### Stack Variables in Playbooks

```yaml
# Stack manifest
components:
  ansible:
    webserver:
      vars:
        app_name: myapp
        app_port: 8080
```

```yaml
# Playbook references variables directly
- name: Deploy app
  hosts: webservers
  tasks:
    - name: Show config
      ansible.builtin.debug:
        msg: "Deploying {{ app_name }} on port {{ app_port }}"
```

## Playbook and Inventory Resolution

The playbook and inventory can be specified in two ways. Command-line flags always take precedence over
stack manifest settings.

**Resolution order (highest to lowest priority):**
1. Command-line flags: `--playbook` / `-p` and `--inventory` / `-i`
2. Stack manifest: `settings.ansible.playbook` and `settings.ansible.inventory`

```yaml
# Stack manifest (lower priority)
components:
  ansible:
    webserver:
      settings:
        ansible:
          playbook: site.yml
          inventory: inventory/production
```

```shell
# Command-line override (higher priority)
atmos ansible playbook webserver -s prod --playbook deploy.yml -i inventory/staging
```

## Configuration in atmos.yaml

Configure Ansible behavior in `atmos.yaml`:

```yaml
components:
  ansible:
    # Executable to run
    command: ansible

    # Base path to Ansible components
    base_path: components/ansible
```

### Configuration Reference

- **`command`** -- Executable to run for Ansible commands. Defaults to `ansible`. Supports absolute paths
  (e.g., `/usr/local/bin/ansible` or `/opt/venv/bin/ansible`).
- **`base_path`** -- Directory containing Ansible component directories. Each subdirectory should contain
  playbooks and related files (roles, inventory, etc.).

### Component Directory Structure

```text
components/ansible/
  hello-world/
    site.yml
    inventory.ini
  webserver/
    site.yml
    roles/
      nginx/
    inventory/
      production
      staging
  database/
    deploy.yml
    inventory.ini
```

## Path-Based Component Resolution

Use filesystem paths instead of component names for convenience:

```shell
# Navigate to component directory and use current directory
cd components/ansible/webserver
atmos ansible playbook . -s prod

# Relative path from components/ansible
cd components/ansible
atmos ansible playbook ./webserver -s prod

# From project root with relative path
atmos ansible playbook components/ansible/webserver -s prod

# Combine with other flags
cd components/ansible/webserver
atmos ansible playbook . -s prod --playbook deploy.yml --inventory production
```

**Supported path formats:**
- `.` -- Current directory
- `./component` -- Relative path from current directory
- `../other-component` -- Relative path to sibling directory
- `/absolute/path/to/component` -- Absolute path

**Requirements:**
- Must be inside a component directory under the configured base path.
- Must specify `--stack` flag.
- Component must exist in the specified stack configuration.
- The component path must resolve to a unique component name. If multiple components reference the same
  path, use the unique component name instead.

## Environment Variables

Common environment variables for Ansible components configured via `env`:

- **`ANSIBLE_HOST_KEY_CHECKING`** -- Controls SSH host key verification. Prefer `"true"` in production; only set to `"false"` in ephemeral or development environments with explicit risk acceptance.
- **`ANSIBLE_FORCE_COLOR`** -- Force colored output (set to `true`).
- **`ANSIBLE_CONFIG`** -- Path to Ansible configuration file.
- **`ANSIBLE_VAULT_PASSWORD_FILE`** -- Path to Ansible Vault password file.
- **`ANSIBLE_ROLES_PATH`** -- Additional paths to search for roles.
- **`ANSIBLE_COLLECTIONS_PATH`** -- Paths to search for collections.

```yaml
components:
  ansible:
    webserver:
      env:
        # Only disable host key checking in dev/ephemeral environments (see security note above)
        ANSIBLE_HOST_KEY_CHECKING: "false"
        ANSIBLE_FORCE_COLOR: "true"
        ANSIBLE_VAULT_PASSWORD_FILE: /path/to/vault-password
```

## Native Flag Passthrough

Any flags placed after `--` are passed directly to `ansible-playbook`:

```shell
# Check mode (dry run at Ansible level)
atmos ansible playbook webserver -s prod -- --check

# Verbose output
atmos ansible playbook webserver -s prod -- -vvv

# Limit to specific hosts
atmos ansible playbook webserver -s prod -- --limit "web01,web02"

# Run specific tags
atmos ansible playbook webserver -s prod -- --tags "deploy"

# Skip specific tags
atmos ansible playbook webserver -s prod -- --skip-tags "slow"

# Additional extra variables (on top of those generated by Atmos)
atmos ansible playbook webserver -s prod -- --extra-vars "version=1.2.3"

# Combine multiple native flags
atmos ansible playbook webserver -s prod -- --check --diff --limit web01 -vv
```

## Stack Inheritance and Imports

Ansible components support the same inheritance model as other Atmos component types:

```yaml
# stacks/catalog/hello-world.yaml (shared defaults)
components:
  ansible:
    hello-world:
      vars:
        app_name: my-app
        app_version: "1.0.0"
        app_port: 8080
      settings:
        ansible:
          playbook: site.yml
          inventory: inventory.ini
```

```yaml
# stacks/deploy/dev.yaml (environment override)
import:
  - catalog/hello-world

vars:
  stage: dev

components:
  ansible:
    hello-world:
      vars:
        app_version: "1.0.0-dev"
```

```yaml
# stacks/deploy/prod.yaml (environment override)
import:
  - catalog/hello-world

vars:
  stage: prod

components:
  ansible:
    hello-world:
      vars:
        app_version: "2.0.0"
        app_port: 443
```

## Debugging

### Describe Component

Use `atmos describe component` to see the fully resolved configuration:

```shell
atmos describe component hello-world -s dev --type ansible
```

### Dry Run

Preview what Atmos will do without executing:

```shell
atmos ansible playbook webserver -s prod --dry-run
```

## Best Practices

> **Interactive use only:** `atmos ansible playbook` is intended for interactive operator sessions, not
> headless CI/CD automation. Ensure a human is present to monitor output and respond to any interactive
> prompts. For fully automated pipelines, invoke `ansible-playbook` directly from a CI step.

1. **Use stack manifest settings for playbook configuration.** Define `settings.ansible.playbook` and
   `settings.ansible.inventory` rather than passing flags every time.

2. **Centralize defaults in catalog files.** Define common settings in catalog defaults and override only
   what differs per environment.

3. **Use `depends_on` for ordering.** Define dependencies when playbooks need to run after infrastructure
   is provisioned (e.g., after Terraform components).

4. **Keep playbooks focused.** Create small, task-specific playbooks rather than monolithic automation.

5. **Use `env` for Ansible configuration.** Configure Ansible behavior through environment variables
   rather than `ansible.cfg` for consistency across environments.

6. **Leverage inheritance.** Use abstract components and inheritance for shared playbook configurations
   across environments.

7. **Use `--dry-run` before production runs.** Preview the commands Atmos will execute before running
   against production infrastructure.

## Additional Resources

- For the complete command reference, see [references/commands-reference.md](references/commands-reference.md)
