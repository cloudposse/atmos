# https://www.openpolicyagent.org/docs/latest/policy-language
# https://www.openpolicyagent.org/
# https://blog.openpolicyagent.org/rego-design-principle-1-syntax-should-reflect-real-world-policies-e1a801ab8bfb
# https://github.com/open-policy-agent/library
# https://github.com/open-policy-agent/example-api-authz-go
# https://github.com/open-policy-agent/opa/issues/2104
# https://www.fugue.co/blog/5-tips-for-using-the-rego-language-for-open-policy-agent-opa
# https://medium.com/@agarwalshubhi17/rego-cheat-sheet-5e25faa6eee8

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
