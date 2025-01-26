# 'package atmos' is required in all `atmos` OPA policies
package atmos

errors["for the 'template-functions-test' component, the variable 'name' must be provided on the command line using the '-var' flag"] {
    not input.cli_vars.name
}
