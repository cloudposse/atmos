# Demo Ansible with Atmos

This demo showcases how to use Ansible with Atmos for configuration management and infrastructure automation.

## Overview

This example demonstrates:

- **Ansible Component Structure**: How to organize Ansible playbooks, inventory, and templates within Atmos components
- **Stack-based Configuration**: Different configurations for dev, staging, and prod environments
- **Variable Management**: Using Atmos variables in Ansible playbooks
- **Settings Integration**: Configuring Ansible-specific settings in Atmos manifests
- **Environment Variables**: Setting Ansible environment variables per stack

## Directory Structure

```
demo-ansible/
├── atmos.yaml              # Atmos configuration
├── components/ansible/
│   └── webapp/            # Ansible component for web application
│       ├── site.yml       # Main playbook
│       ├── inventory.yml  # Default inventory
│       ├── ansible.cfg    # Ansible configuration
│       ├── templates/     # Jinja2 templates
│       └── group_vars/    # Group variables
├── stacks/
│   ├── catalog/
│   │   └── demo.yaml      # Component catalog
│   └── deploy/
│       ├── dev/demo.yaml      # Development environment
│       ├── staging/demo.yaml  # Staging environment
│       └── prod/demo.yaml     # Production environment
└── schemas/
    └── atmos-manifest.json    # Schema validation
```

## Features Demonstrated

### 1. Component Configuration

The webapp component (`stacks/catalog/demo.yaml`) shows:

- **Variable definitions** for application configuration
- **Settings section** for Ansible-specific configuration
- **Environment variables** for Ansible behavior
- **Metadata** for component description and type

### 2. Environment-specific Overrides

Each environment (`dev/staging/prod`) demonstrates:

- **Different variable values** per environment
- **Environment-specific Ansible settings**
- **Custom environment variables** for different stages

### 3. Ansible Integration

- **Playbook execution** with component variables
- **Inventory management** with dynamic host targeting
- **Template rendering** using Atmos variables
- **Vault operations** for secret management

## Quick Start

### Prerequisites

- Atmos CLI installed
- Ansible installed (`pip install ansible`)
- Access to target hosts (or use `--check` mode for dry run)

### Running the Demo

1. **Navigate to the demo directory:**
   ```bash
   cd examples/demo-ansible
   ```

2. **Validate the stack configuration:**
   ```bash
   atmos validate stacks
   ```

3. **List available components:**
   ```bash
   atmos list components
   ```

4. **Describe the component configuration:**
   ```bash
   atmos describe component webapp -s dev
   ```

5. **View the inventory:**
   ```bash
   atmos ansible inventory webapp -s dev --list
   ```

6. **Run the playbook (dry run):**
   ```bash
   atmos ansible playbook webapp -s dev -- --check
   ```

7. **Run the playbook:**
   ```bash
   atmos ansible playbook webapp -s dev
   ```

### Advanced Examples

```bash
# Check differences between environments
atmos describe component webapp -s dev
atmos describe component webapp -s prod

# Use custom playbook
atmos ansible playbook webapp -s dev --playbook custom-site.yml

# Run with specific inventory
atmos ansible playbook webapp -s prod --inventory production-hosts.yml

# Vault operations
atmos ansible vault webapp -s dev encrypt secrets.yml
atmos ansible vault webapp -s dev view secrets.yml

# Custom commands defined in atmos.yaml
atmos demo                    # Run full demo workflow
atmos ansible test           # Test all environments
atmos ansible deploy        # Deploy to all environments
```

## Configuration Highlights

### Variable Inheritance

Variables are inherited and overridden through the stack hierarchy:

- **Base catalog** (`catalog/demo.yaml`): Default values
- **Environment stacks** (`deploy/*/demo.yaml`): Environment-specific overrides

### Ansible Settings

The `settings.ansible` section allows you to configure:

- `playbook`: Default playbook file
- `inventory`: Default inventory file
- Additional Ansible-specific configuration

### Environment Variables

Ansible behavior can be customized using environment variables:

- `ANSIBLE_HOST_KEY_CHECKING`: Disable host key checking
- `ANSIBLE_STDOUT_CALLBACK`: Set output format
- `ANSIBLE_VERBOSITY`: Control output verbosity

## Integration with Atmos Features

This demo showcases integration with core Atmos features:

- **Stack inheritance** and component composition
- **Variable templating** with Go templates
- **Schema validation** for configuration files
- **Custom commands** for workflow automation
- **Multi-environment** configuration management

## Next Steps

Try customizing the demo by:

1. **Adding new playbook tasks** to the `site.yml` file
2. **Creating additional components** for different services
3. **Modifying inventory** to match your infrastructure
4. **Adding custom variables** for your specific use case
5. **Creating environment-specific playbooks** for complex deployments

This demo provides a foundation for using Ansible with Atmos in production environments.