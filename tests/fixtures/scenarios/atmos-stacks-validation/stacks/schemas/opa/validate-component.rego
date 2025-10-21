# 'package atmos' is required in all Atmos OPA policies
package atmos

# Atmos looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, Atmos considers the policy failed

# Test 1: Don't allow `terraform apply` if the `foo` variable is set to `foo`
# The `input` map contains the `cli_args` attribute (a list of the command line arguments and flags)
errors[message] {
    count(input.cli_args) >= 2
    input.cli_args[0] == "terraform"
    input.cli_args[1] == "apply"
    input.vars.foo == "foo"
    message = "the component can't be applied if the 'foo' variable is set to 'foo'"
}

# Test 2: Validate that process_env section exists and contains required environment variables
errors[message] {
    not input.process_env
    message = "process_env section is missing from component configuration"
}

errors[message] {
    count(input.process_env) == 0
    message = "process_env section is empty"
}

# Test 3: Validate specific environment variables exist in process_env
errors[message] {
    not input.process_env.PATH
    message = "PATH environment variable is missing from process_env"
}

# Test 4: Validate cli_args structure and content
errors[message] {
    not input.cli_args
    message = "cli_args section is missing from component configuration"
}

errors[message] {
    count(input.cli_args) < 2
    input.vars.test_cli_args == true
    message = "cli_args must contain at least 2 arguments when test_cli_args is enabled"
}

# Test 5: Validate tf_cli_vars when present
errors[message] {
    input.tf_cli_vars
    not is_object(input.tf_cli_vars)
    message = "tf_cli_vars must be an object/map when present"
}

# Test 6: Validate env_tf_cli_args when TF_CLI_ARGS is set
errors[message] {
    input.env_tf_cli_args
    not is_array(input.env_tf_cli_args)
    message = "env_tf_cli_args must be an array when present"
}

# Test 7: Validate env_tf_cli_vars when TF_CLI_ARGS contains variables
errors[message] {
    input.env_tf_cli_vars
    not is_object(input.env_tf_cli_vars)
    message = "env_tf_cli_vars must be an object/map when present"
}

# Test 8: Ensure process_env contains expected test variables when in test mode
errors[message] {
    input.vars.test_mode == true
    not input.process_env.ATMOS_TEST_VAR
    message = "ATMOS_TEST_VAR environment variable is missing from process_env in test mode"
}

# Test 9: Validate cli_args contains expected terraform commands
errors[message] {
    input.vars.validate_terraform_commands == true
    count(input.cli_args) >= 1
    input.cli_args[0] != "terraform"
    message = "First cli_arg must be 'terraform' when validate_terraform_commands is enabled"
}

# Test 10: Check TF_CLI_ARGS processing when test variables are provided
errors[message] {
    input.vars.test_tf_cli_vars == true
    input.env_tf_cli_vars
    not input.env_tf_cli_vars.test_var
    message = "test_var is missing from env_tf_cli_vars when test_tf_cli_vars is enabled"
}
