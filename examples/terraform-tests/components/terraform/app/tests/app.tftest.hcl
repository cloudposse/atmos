# Terraform tests for the `app` component. The `command = apply` run blocks
# create real AWS resources — normally that requires a cloud account and spend.
# Here Atmos points the AWS provider at a local emulator (Floci), so these tests
# run for free and hermetically on a laptop. The component hooks on
# before/after.terraform.test (see stacks/catalog/app.yaml) start the emulator,
# provision the fixture VPC, then tear both down automatically. Atmos passes
# stack `test.vars` to Terraform test, so this file can declare test-scope
# variables sourced from the fixture state. Run: `atmos terraform test app -s fixtures`.

variable "expected_bucket_name" {
  type = string
}

variable "fixture_vpc_id" {
  type = string
}

variables {
  name             = "atmos-demo"
  environment      = "test"
  fixture_vpc_name = "atmos-fixture-vpc"
}

# Static validation — runs as `plan`, no infrastructure created.
run "bucket_name_is_namespaced" {
  command = plan

  assert {
    condition     = aws_s3_bucket.this.bucket == var.expected_bucket_name
    error_message = "Bucket name should combine the name and environment variables"
  }

  assert {
    condition     = data.aws_vpc.fixture.tags.Name == "atmos-fixture-vpc"
    error_message = "The app component should resolve the fixture VPC by tag"
  }
}

# Real apply against the emulator — creates the bucket, versioning, and table,
# then asserts on the outputs. The VPC is not created by this component; it is
# provisioned by the hook from the fixture component before Terraform test runs.
run "provisions_resources_against_emulator" {
  command = apply

  assert {
    condition     = output.fixture_vpc_cidr_block == "10.99.0.0/16"
    error_message = "The app component did not read the fixture VPC"
  }

  assert {
    condition     = output.fixture_vpc_id == var.fixture_vpc_id
    error_message = "The app component did not read the fixture VPC ID from fixture state"
  }

  assert {
    condition     = output.bucket_id == var.expected_bucket_name
    error_message = "The S3 bucket was not created against the emulator"
  }

  assert {
    condition     = output.table_name == var.expected_bucket_name
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
