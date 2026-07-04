# This file intentionally does not declare or reference the test-scope variables
# from stack `test.vars`. Terraform should ignore those values for this file
# without undeclared-variable warnings.

run "ignores_fixture_test_vars" {
  command = plan

  assert {
    condition     = aws_s3_bucket.this.bucket == "atmos-demo-fixtures"
    error_message = "The component stack vars should still apply when test vars are ignored"
  }
}
