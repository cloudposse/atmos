# https://www.openpolicyagent.org/docs/v0.13.5/how-do-i-write-policies
# https://www.openpolicyagent.org/docs/latest/policy-language
# https://www.openpolicyagent.org/docs/latest/policy-reference
# https://www.openpolicyagent.org/docs/v0.12.2/language-reference/#regex

# 'atmos' looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, 'atmos' considers the policy failed

# 'package atmos' is required in all 'atmos' OPA policies
package atmos

# Function `object_has_key` checks if an object has the specified key with a string value
# https://www.openpolicyagent.org/docs/latest/policy-reference/#types
object_has_key(o, k) {
    some item
    item = o[k]
    type_name(item) == "string"
}

# Check the app hostname using Regex
errors[message] {
    not re_match("^([a-z0-9]+([\\-a-z0-9]*[a-z0-9]+)?\\.){1,}([a-z0-9]+([\\-a-z0-9]*[a-z0-9]+)?){1,63}(\\.[a-z0-9]{2,7})+$", input.vars.app_config.hostname)
    message = "'app_config.hostname' must contain at least a subdomain and a top level domain. Example: subDomain1.topLevelDomain.com"
}

# Check the email address using Regex
errors[message] {
    not re_match("^([a-zA-Z0-9_\\-\\.]+)@([a-zA-Z0-9_\\-\\.]+)\\.([a-zA-Z]{2,5})$", input.vars.app_config.contact.email)
    message = "'app_config.contact.email' must be a valid email address"
}

# Check the phone number using Regex
errors[message] {
    not re_match("^[\\+]?[(]?[0-9]{3}[)]?[-\\s\\.]?[0-9]{3}[-\\s\\.]?[0-9]{4,6}", input.vars.app_config.contact.phone)
    message = "'app_config.contact.phone' must be a valid phone number"
}

# Check if the component has a `Team` tag
errors[message] {
    not object_has_key(input.vars.tags, "Team")
    message = "All components must have 'Team' tag defined to specify which team is responsible for managing and provisioning them"
}

# Check if the Team has permissions to provision components in an OU (tenant)
errors[message] {
    input.vars.tags.Team == "devs"
    input.vars.tenant == "corp"
    message = "'devs' team cannot provision components into 'corp' OU"
}

# Check the message of the day from the manager
# If `settings.notes.allowed` is set to `false`, output the message from the manager
errors[message] {
    input.settings.notes.allowed == false
    message = concat("", [input.settings.notes.manager, " says: ", input.settings.notes.message])
}

# Check `notes2` config in the free-form Atmos section `settings`
errors[message] {
    input.settings.notes2.message == ""
    message = "'notes2.message' should not be empty"
}

# Check that the `app_config.hostname` variable is defined only once for the stack across all stack config files
# Refer to https://atmos.tools/cli/commands/describe/component#sources-of-component-variables for details on how 'atmos' detects sources for all variables
# https://www.openpolicyagent.org/docs/latest/policy-language/#universal-quantification-for-all
errors[message] {
    hostnames := {app_config | some app_config in input.sources.vars.app_config; app_config.hostname}
    count(hostnames) > 0
    message = "'app_config.hostname' variable must be defined only once for the stack across all stack config files"
}

# This policy checks that the 'bar' variable is not defined in any of the '_defaults.yaml' Atmos stack config files
# Refer to https://atmos.tools/cli/commands/describe/component#sources-of-component-variables for details on how 'atmos' detects sources for all variables
# https://www.openpolicyagent.org/docs/latest/policy-language/#universal-quantification-for-all
errors[message] {
    # Get all 'stack_dependencies' of the 'bar' variable
    stack_dependencies := input.sources.vars.bar.stack_dependencies
    # Get all stack dependencies of the 'bar' variable where 'stack_file' ends with '_defaults'
    defaults_stack_dependencies := {stack_dependency | some stack_dependency in stack_dependencies; endswith(stack_dependency.stack_file, "_defaults")}
    # Check the count of the stack dependencies of the 'bar' variable where 'stack_file' ends with '_defaults'
    count(defaults_stack_dependencies) > 0
    # Generate the error message
    message = "The 'bar' variable must not be defined in any of '_defaults.yaml' stack config files"
}

# This policy checks that if the 'foo' variable is defined in the 'stack1.yaml' stack config file, it cannot be overridden in 'stack2.yaml'
# Refer to https://atmos.tools/cli/commands/describe/component#sources-of-component-variables for details on how 'atmos' detects sources for all variables
# https://www.openpolicyagent.org/docs/latest/policy-language/#universal-quantification-for-all
errors[message] {
    # Get all 'stack_dependencies' of the 'foo' variable
    stack_dependencies := input.sources.vars.foo.stack_dependencies
    # Check if the 'foo' variable is defined in the 'stack1.yaml' stack config file
    stack1_dependency := endswith(stack_dependencies[0].stack_file, "stack1")
    stack1_dependency == true
    # Get all stack dependencies of the 'foo' variable where 'stack_file' ends with 'stack2' (this means that the variable is redefined in one of the files 'stack2')
    stack2_dependencies := {stack_dependency | some stack_dependency in stack_dependencies; endswith(stack_dependency.stack_file, "stack2")}
    # Check the count of the stack dependencies of the 'foo' variable where 'stack_file' ends with 'stack2'
    count(stack2_dependencies) > 0
    # Generate the error message
    message = "If the 'foo' variable is defined in 'stack1.yaml', it cannot be overridden in 'stack2.yaml'"
}

# This policy shows an example on how to check the imported files in the stacks
# All stack files (root stacks and imported) that the current component depends on are in the `deps` section
# For example:
# deps:
# - catalog/xxx
# - catalog/yyy
# - orgs/zzz/_defaults
errors[message] {
    input.vars.tags.Team == "devs"
    input.vars.tenant == "corp"
    input.deps[_] == "catalog/xxx"
    message = "'devs' team cannot import the 'catalog/xxx' file when provisioning components into 'corp' OU"
}

# Note:
# If a regex pattern in the 're_match' function contains a backslash to escape special chars (e.g. '\.' or '\-'),
# it must be escaped with another backslash when represented as a regular Go string ('\\.', '\\-').
# The reason is that backslash is also used to escape special characters in Go strings like newline (\n).
# If you want to match the backslash character itself, you'll need four slashes.
