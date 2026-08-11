# https://www.openpolicyagent.org/docs/latest/policy-language
# https://www.openpolicyagent.org/

# Atmos looks for the 'errors' (array of strings) output from all OPA policies.
# If the 'errors' output contains one or more error messages, Atmos considers the policy failed.

# 'package atmos' is required in all `atmos` OPA policies.
package atmos

# The bucket `name` variable is required.
errors[message] {
    not input.vars.name
    message := "The 's3-bucket' 'name' variable must be set"
}

# The bucket `name` variable must not exceed 40 characters.
errors[message] {
    count(input.vars.name) > 40
    message := "The 's3-bucket' 'name' variable must not be longer than 40 characters"
}

# In production, S3 buckets must have versioning enabled.
errors[message] {
    input.vars.stage == "prod"
    input.vars.versioning_enabled != true
    message := "In 'prod', S3 buckets must have versioning enabled. Set 'versioning_enabled' variable to 'true'"
}
