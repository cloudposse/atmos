# 'package atmos' is required in all Atmos OPA policies
package atmos

# Atmos looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, Atmos considers the policy failed

# Don't allow `terraform apply` if the `foo` variable is set to `foo`
# The `input` map contains the `cli_args` attribute (a list of the command line arguments and flags)
errors[message] {
    count(input.cli_args) >= 2
    input.cli_args[0] == "terraform"
    input.cli_args[1] == "apply"
    input.vars.foo == "foo"
    message = "the component can't be applied if the 'foo' variable is set to 'foo'"
}
