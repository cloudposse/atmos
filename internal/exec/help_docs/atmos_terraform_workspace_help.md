# atmos terraform workspace

Check out the ['atmos terraform workspace' documentation](https://atmos.tools/cli/commands/terraform/workspace).

## Description

`atmos terraform workspace` command calculates the Terraform workspace for an Atmos component in a stack, then
executes `terraform init -reconfigure` and selects the Terraform workspace by executing the `terraform workspace
select` command.

If the workspace does not exist, the command creates it by executing the `terraform workspace new` command.

## Examples

`atmos terraform workspace vpc-flow-logs-bucket --stack plat-ue2-prod`

`atmos terraform workspace vpc -s plat-ue2-dev`

<br/>
