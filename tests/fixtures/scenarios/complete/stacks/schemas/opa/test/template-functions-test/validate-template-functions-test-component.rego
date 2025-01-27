# 'package atmos' is required in all `atmos` OPA policies
package atmos

# Atmos looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, Atmos considers the policy failed

errors["for the 'template-functions-test' component, the variable 'name' must be provided on the command line using the '-var' flag"] {
    not input.tf_cli_vars.name
}

# https://www.openpolicyagent.org/docs/latest/policy-language
# https://www.openpolicyagent.org/
# https://blog.openpolicyagent.org/rego-design-principle-1-syntax-should-reflect-real-world-policies-e1a801ab8bfb
# https://github.com/open-policy-agent/library
# https://github.com/open-policy-agent/example-api-authz-go
# https://github.com/open-policy-agent/opa/issues/2104
# https://www.fugue.co/blog/5-tips-for-using-the-rego-language-for-open-policy-agent-opa
# https://medium.com/@agarwalshubhi17/rego-cheat-sheet-5e25faa6eee8
# https://code.tutsplus.com/tutorials/regular-expressions-with-go-part-1--cms-30403
# https://www.styra.com/blog/how-to-write-your-first-rules-in-rego-the-policy-language-for-opa
# https://www.openpolicyagent.org/docs/v0.12.2/how-does-opa-work
