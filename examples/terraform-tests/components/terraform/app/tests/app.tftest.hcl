# Terraform tests for the `app` component. The `command = apply` run blocks
# create real AWS resources — normally that requires a cloud account and spend.
# Here Atmos points the AWS provider at a local emulator (Floci), so these tests
# run for free and hermetically on a laptop. The component hooks on
# before/after.terraform.test (see stacks/catalog/app.yaml) start and stop the
# emulator automatically, so `atmos terraform test app -s local` is all you run.

variables {
  name        = "atmos-demo"
  environment = "test"
}

# Static validation — runs as `plan`, no infrastructure created.
run "bucket_name_is_namespaced" {
  command = plan

  assert {
    condition     = aws_s3_bucket.this.bucket == "atmos-demo-test"
    error_message = "Bucket name should combine the name and environment variables"
  }
}

# Real apply against the emulator — creates the bucket, versioning, and table,
# then asserts on the outputs. These pass only if the resources were actually
# created in the emulator.
run "provisions_resources_against_emulator" {
  command = apply

  assert {
    condition     = output.bucket_id == "atmos-demo-test"
    error_message = "The S3 bucket was not created against the emulator"
  }

  assert {
    condition     = output.table_name == "atmos-demo-test"
    error_message = "The DynamoDB table was not created against the emulator"
  }

  assert {
    condition     = output.versioning_status == "Enabled"
    error_message = "Versioning should be enabled by default"
  }
}

# Override a variable for a single run and re-apply to verify the toggle.
run "versioning_can_be_disabled" {
  command = apply

  variables {
    enable_versioning = false
  }

  assert {
    condition     = output.versioning_status == "Suspended"
    error_message = "Versioning should be suspended when enable_versioning is false"
  }
}
