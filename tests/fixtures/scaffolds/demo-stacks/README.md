# Demo Stacks

This example demonstrates how to use Atmos stacks to manage infrastructure components.

## Structure

- **stacks/**: Contains stack configurations
- **components/**: Contains the actual infrastructure code

## Usage

1. **Describe the stack**:
   ```bash
   atmos describe stack dev-tenant -f stacks/orgs/cp/tenant/terraform/dev/us-east-2.yaml
   ```

2. **Plan the VPC component**:
   ```bash
   atmos terraform plan vpc -s dev-tenant -f stacks/orgs/cp/tenant/terraform/dev/us-east-2.yaml
   ```

3. **Apply the VPC component**:
   ```bash
   atmos terraform apply vpc -s dev-tenant -f stacks/orgs/cp/tenant/terraform/dev/us-east-2.yaml
   ```

## Key Concepts

- **Stacks**: Define environment-specific configurations
- **Components**: Reusable infrastructure modules
- **Variables**: Environment-specific values
- **Imports**: Share common configurations across stacks
