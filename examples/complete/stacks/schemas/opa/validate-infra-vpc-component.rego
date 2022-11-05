# https://www.openpolicyagent.org/docs/latest/policy-language
# https://www.openpolicyagent.org/
# https://blog.openpolicyagent.org/rego-design-principle-1-syntax-should-reflect-real-world-policies-e1a801ab8bfb
# https://github.com/open-policy-agent/library
# https://github.com/open-policy-agent/example-api-authz-go
# https://github.com/open-policy-agent/opa/issues/2104
# https://www.fugue.co/blog/5-tips-for-using-the-rego-language-for-open-policy-agent-opa
# https://medium.com/@agarwalshubhi17/rego-cheat-sheet-5e25faa6eee8
# https://code.tutsplus.com/tutorials/regular-expressions-with-go-part-1--cms-30403

# 'atmos' looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, 'atmos' considers the policy failed

# 'package atmos' is required in all `atmos` OPA policies
package atmos

# In production, don't allow mapping public IPs on launch
errors[message] {
    input.vars.stage == "prod"
    input.vars.map_public_ip_on_launch == true
    message = "Mapping public IPs on launch is not allowed in 'prod'. Set 'map_public_ip_on_launch' variable to 'false'"
}

# In 'dev', only 2 Availability Zones are allowed
errors[message] {
    input.vars.stage == "dev"
    count(input.vars.availability_zones) != 2
    message = "In 'dev', only 2 Availability Zones are allowed"
}

# Check VPC name
errors[message] {
    not re_match("^[a-zA-Z0-9]{2,20}$", input.vars.name)
    message = "VPC name must be a valid string from 2 to 20 alphanumeric chars"
}

# Note:
# If a regex pattern in the `re_match` function contains a backslash to escape special chars (e.g. `\.` or `\-`),
# it must be escaped with another backslash when represented as a regular Go string (`\\.`, `\\-`).
# The reason is that backslash is also used to escape special characters in Go strings like newline (\n).
# If you want to match the backslash character itself, you'll need four slashes.
