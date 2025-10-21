# Check for available version updates without making changes
atmos vendor update --check

# Update version references in vendor configuration files
atmos vendor update

# Update version references AND pull the new component versions
atmos vendor update --pull

# Update version for a specific component
atmos vendor update --component vpc
atmos vendor update -c vpc-flow-logs-bucket

# Update versions for components with specific tags
atmos vendor update --tags terraform
atmos vendor update --tags networking,storage

# Combine flags for specific workflows
atmos vendor update --component vpc --pull
atmos vendor update --tags terraform --check
