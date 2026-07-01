# Atmos OPA policy for the 'bucket' component.
# Atmos reads the 'errors' (array of strings) output; a non-empty array fails validation.

# 'package atmos' is required in all Atmos OPA policies.
package atmos

# The bucket name must be present.
errors[message] {
    not input.vars.name
    message := "The 'bucket' component requires a non-empty 'name' variable."
}

# The bucket name must not be blank.
errors[message] {
    input.vars.name == ""
    message := "The 'bucket' component requires a non-empty 'name' variable."
}
