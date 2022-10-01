# https://www.openpolicyagent.org/docs/latest/policy-language
# https://www.openpolicyagent.org/
# https://blog.openpolicyagent.org/rego-design-principle-1-syntax-should-reflect-real-world-policies-e1a801ab8bfb
# https://github.com/open-policy-agent/library
# https://github.com/open-policy-agent/example-api-authz-go
# https://github.com/open-policy-agent/opa/issues/2104
# https://www.fugue.co/blog/5-tips-for-using-the-rego-language-for-open-policy-agent-opa

# 'package atmos' is required in all `atmos` OPA policies
package atmos

default allow := true

# In production, don't allow mapping public IPs on launch
errors[message] {
    input.vars.stage == "prod"
    input.vars.map_public_ip_on_launch == true
    message = "Mapping public IPs on launch is not allowed in 'prod'"
}

# In 'sandbox', only 2 AZs are allowed
errors[message] {
    input.vars.stage == "sandbox"
    count(input.vars.availability_zones) != 2
    message = "In 'sandbox', only 2 AZs are allowed"
}

# 'atmos' looks for the 'allow' output (boolean) from OPA policies
allow := false {
    count(errors) > 0
}
