# 'package atmos' is required in all `atmos` OPA policies
package atmos

# Atmos looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, Atmos considers the policy failed

# Don't allow `terraform apply` if the `foo` variable is set to `foo`
# The `input` variable contains the following attributes:
# - `cli_command` - Atmos command, e.g. `terraform`, `helmfile`
# - `cli_subcommand` -  subcommand, e.g. `apply`, `plan`, `generate` (as in `atmos terraform apply`)
# - `cli_subcommand2` -  subcommand2, e.g. `varfile`, `varfiles`, `planfile` (as in `atmos terraform generate varfile`)
errors[message] {
    input.cli_command == "terraform"
    input.cli_subcommand == "apply"
    input.vars.foo == "foo"
    message = "the component can't be applied if the 'foo' variable is set to 'foo'"
}
