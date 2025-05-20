# 'package atmos' is required in all `atmos` OPA policies
package atmos

# Atmos looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, Atmos considers the policy failed

# Don't allow `terraform apply` if the `foo` variable is set to `foo`
errors[message] {
    input.cli_command == "terraform"
    input.cli_subcommand == "apply"
    input.vars.foo == "foo"
    message = "'devs' team cannot provision components into 'corp' OU"
}
