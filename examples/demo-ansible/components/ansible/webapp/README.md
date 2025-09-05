# Webapp Ansible Component

This is a demo Ansible component that configures a simple web application using nginx.

## Files

- `site.yml` - Main playbook for webapp deployment
- `inventory.yml` - Default inventory with webservers and load balancers
- `templates/app.conf.j2` - Nginx configuration template

## Configuration

The component accepts the following variables:

- `webapp_name` - Name of the web application (default: mywebapp)
- `webapp_version` - Version of the application (default: 1.0.0)
- `webapp_port` - Port to run the application on (default: 8080)
- `environment` - Environment name (dev/staging/prod)
- `target_hosts` - Ansible host group to target (default: webservers)

## Usage

```bash
# Run playbook against dev stack
atmos ansible playbook webapp --stack dev

# Use custom inventory
atmos ansible playbook webapp --stack dev --inventory custom-hosts.yml

# Check what would be changed (dry run)
atmos ansible playbook webapp --stack dev -- --check

# Display inventory
atmos ansible inventory webapp --stack dev --list
```