# Atmos Ansible Commands Reference

Complete reference of all `atmos ansible` subcommands with syntax, flags, and examples.

## Command Syntax

```shell
atmos ansible <subcommand> [<component>] [-s <stack>] [flags] [-- native-ansible-flags]
```

The `component` argument and `--stack` / `-s` flag are required for the `playbook` subcommand.
Use `--` to pass flags directly to the underlying Ansible command without Atmos interpretation.

## Subcommands

### playbook

Run an Ansible playbook for a component in a stack. This is the primary command for executing
configuration management operations.

```shell
atmos ansible playbook <component> -s <stack> [flags] [-- ansible-options]
```

**How it works:**

1. Resolves the full stack configuration for the component.
2. Generates a YAML variables file from the `vars` section.
3. Determines the playbook from `--playbook` flag or `settings.ansible.playbook`.
4. Determines the inventory from `--inventory` flag or `settings.ansible.inventory`.
5. Sets environment variables from the `env` section.
6. Runs `ansible-playbook` in the component directory with `--extra-vars @<varfile>`.
7. Cleans up the generated variables file.

#### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `component` | Yes | Atmos Ansible component name or filesystem path |

The component can be specified as:
- **Component name**: `webserver`, `database/postgres`
- **Filesystem path**: `.` (current directory), `./webserver`, `../sibling`, `/absolute/path`

When using a filesystem path, Atmos resolves it to the component name based on the stack configuration.
The path must be within the configured `base_path` and must resolve to a unique component name.

#### Flags

| Flag | Alias | Required | Description |
|------|-------|----------|-------------|
| `--stack` | `-s` | Yes | Atmos stack to target |
| `--playbook` | `-p` | No | Playbook file to execute. Overrides `settings.ansible.playbook` from stack manifest |
| `--inventory` | `-i` | No | Inventory source (file, directory, or dynamic script). Overrides `settings.ansible.inventory` from stack manifest |
| `--dry-run` | | No | Show the commands that would be executed without running them |

#### Examples

```shell
# Basic playbook execution using stack manifest settings
atmos ansible playbook webserver --stack prod

# Short form for stack flag
atmos ansible playbook webserver -s prod

# Override the playbook file
atmos ansible playbook webserver -s prod --playbook deploy.yml

# Override both playbook and inventory
atmos ansible playbook webserver -s prod -p site.yml -i inventory/production

# Dry run to preview commands
atmos ansible playbook webserver -s prod --dry-run
```

#### Path-Based Examples

```shell
# Use current directory as component reference
cd components/ansible/webserver
atmos ansible playbook . -s prod

# Relative path from components/ansible
cd components/ansible
atmos ansible playbook ./webserver -s prod

# From project root
atmos ansible playbook components/ansible/webserver -s prod

# Combine path with flag overrides
cd components/ansible/webserver
atmos ansible playbook . -s prod --playbook deploy.yml --inventory production
```

#### Native Ansible Flag Passthrough

All flags after `--` are passed directly to `ansible-playbook`:

```shell
# Check mode (Ansible-level dry run, no changes made)
atmos ansible playbook webserver -s prod -- --check

# Check mode with diff output
atmos ansible playbook webserver -s prod -- --check --diff

# Verbose output (increasing verbosity levels)
atmos ansible playbook webserver -s prod -- -v
atmos ansible playbook webserver -s prod -- -vv
atmos ansible playbook webserver -s prod -- -vvv
atmos ansible playbook webserver -s prod -- -vvvv

# Limit execution to specific hosts
atmos ansible playbook webserver -s prod -- --limit web01
atmos ansible playbook webserver -s prod -- --limit "web01,web02"
atmos ansible playbook webserver -s prod -- --limit "webservers:&us-east-1"

# Run only tasks with specific tags
atmos ansible playbook webserver -s prod -- --tags "deploy"
atmos ansible playbook webserver -s prod -- --tags "deploy,config"

# Skip tasks with specific tags
atmos ansible playbook webserver -s prod -- --skip-tags "slow"
atmos ansible playbook webserver -s prod -- --skip-tags "slow,backup"

# Pass additional extra variables (on top of Atmos-generated vars)
atmos ansible playbook webserver -s prod -- --extra-vars "version=1.2.3"
atmos ansible playbook webserver -s prod -- --extra-vars "version=1.2.3 region=us-east-1"

# Start at a specific task
atmos ansible playbook webserver -s prod -- --start-at-task "Deploy application"

# Step through tasks one at a time
atmos ansible playbook webserver -s prod -- --step

# Set the number of parallel processes (forks)
atmos ansible playbook webserver -s prod -- --forks 10

# Specify a vault password file
atmos ansible playbook webserver -s prod -- --vault-password-file /path/to/vault-pass

# Combine multiple native flags
atmos ansible playbook webserver -s prod -- --check --diff --limit web01 -vv --tags deploy
```

#### Common Native Ansible-Playbook Flags

| Flag | Description |
|------|-------------|
| `--check` | Run in check mode (no changes, predict changes) |
| `--diff` | Show differences in changed files |
| `-v` / `-vv` / `-vvv` / `-vvvv` | Increase verbosity level |
| `--limit <pattern>` | Limit execution to matching hosts |
| `--tags <tags>` | Only run plays and tasks tagged with these values |
| `--skip-tags <tags>` | Skip plays and tasks tagged with these values |
| `--extra-vars <vars>` | Additional variables as key=value or @file |
| `--start-at-task <name>` | Start at the named task |
| `--step` | Confirm each task before running |
| `--forks <num>` | Number of parallel processes (default: 5) |
| `--vault-password-file <file>` | Vault password file |
| `--ask-vault-pass` | Prompt for vault password |
| `--become` | Run operations with privilege escalation |
| `--become-user <user>` | Privilege escalation user (default: root) |
| `--become-method <method>` | Privilege escalation method (default: sudo) |
| `--private-key <file>` | SSH private key file |
| `--user <user>` | Connect as this user |
| `--connection <type>` | Connection type (ssh, local, etc.) |
| `--timeout <seconds>` | Connection timeout |
| `--syntax-check` | Perform a syntax check on the playbook |
| `--list-tasks` | List all tasks that would be executed |
| `--list-hosts` | List all hosts that would be targeted |
| `--list-tags` | List all available tags |

---

### version

Display the installed Ansible version and configuration information. This command takes no arguments
and does not require a component or stack.

```shell
atmos ansible version
```

**Output includes:**
- Ansible core version
- Configuration file location
- Configured module search path
- Python version and location
- Ansible collection location
- Jinja2 version

#### Arguments

This command takes no arguments.

#### Flags

| Flag | Description |
|------|-------------|
| `--help` | Display help for the command |

#### Example

```shell
atmos ansible version
```

Example output:

```text
ansible [core 2.15.0]
  config file = /etc/ansible/ansible.cfg
  configured module search path = ['/home/user/.ansible/plugins/modules', '/usr/share/ansible/plugins/modules']
  ansible python module location = /usr/lib/python3/dist-packages/ansible
  ansible collection location = /home/user/.ansible/collections:/usr/share/ansible/collections
  executable location = /usr/bin/ansible
  python version = 3.11.2 (main, Mar 13 2023, 12:18:29) [GCC 12.2.0]
  jinja version = 3.1.2
  libyaml = True
```

---

## Stack Manifest Configuration

### Component Configuration

```yaml
components:
  ansible:
    <component_name>:
      vars: {}               # Variables passed via --extra-vars @<varfile>
      env: {}                # Environment variables set during execution
      settings:
        ansible:
          playbook: <file>   # Playbook file (relative to component directory)
          inventory: <src>   # Inventory source (file, directory, or script)
        depends_on: []       # Dependency ordering
      metadata: {}           # Component behavior and inheritance
      command: ansible       # Override the ansible binary
      hooks: {}              # Lifecycle event handlers
```

### Variable File Generation

Atmos generates a YAML file from `vars` and passes it via `--extra-vars @<filename>`.

**Naming convention:** `<context>-<component>.ansible.vars.yaml`

Example: For component `webserver` with context `acme-plat-prod-us-east-1`:
```text
acme-plat-prod-us-east-1-webserver.ansible.vars.yaml
```

The file is automatically cleaned up after playbook execution.

### Precedence for Playbook and Inventory

1. **Command-line flags** (highest priority): `--playbook` / `-p`, `--inventory` / `-i`
2. **Stack manifest settings**: `settings.ansible.playbook`, `settings.ansible.inventory`

---

## atmos.yaml Configuration

Configure Ansible behavior globally in `atmos.yaml`:

```yaml
components:
  ansible:
    command: ansible               # Executable name or path
    base_path: components/ansible  # Base directory for Ansible components
```

| Setting | Default | Description |
|---------|---------|-------------|
| `command` | `ansible` | The Ansible executable. Can be a name on PATH or an absolute path |
| `base_path` | `components/ansible` | Directory containing Ansible component subdirectories |

---

## Common Patterns

### Development Workflow

```shell
# Check Ansible is available
atmos ansible version

# Preview the playbook execution
atmos ansible playbook webserver -s dev --dry-run

# Run with check mode first (Ansible dry run)
atmos ansible playbook webserver -s dev -- --check --diff

# Execute the playbook
atmos ansible playbook webserver -s dev

# Run with verbose output for debugging
atmos ansible playbook webserver -s dev -- -vvv
```

### Production Workflow

```shell
# Verify configuration
atmos describe component webserver -s prod --type ansible

# Run check mode against production
atmos ansible playbook webserver -s prod -- --check --diff

# Execute against a limited set of hosts first
atmos ansible playbook webserver -s prod -- --limit "web01"

# Execute against all hosts
atmos ansible playbook webserver -s prod
```

### Tag-Based Execution

```shell
# Only deploy application
atmos ansible playbook webserver -s prod -- --tags "deploy"

# Only update configuration
atmos ansible playbook webserver -s prod -- --tags "config"

# Skip slow tasks during development
atmos ansible playbook webserver -s dev -- --skip-tags "slow,backup"
```

### Inventory Overrides

```shell
# Use staging inventory against prod config
atmos ansible playbook webserver -s prod -i inventory/staging

# Use a dynamic inventory script
atmos ansible playbook webserver -s prod -i scripts/aws_inventory.py

# Use a directory of inventory sources
atmos ansible playbook webserver -s prod -i inventory/
```
