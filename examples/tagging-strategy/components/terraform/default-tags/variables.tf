# This mock component declares no provider requirements on purpose, so the example runs
# without credentials or network access. The `aws` provider block, including its
# `default_tags`, is injected by Atmos into `providers_override.tf.json` from the
# `terraform.providers.aws.default_tags.tags` section of the stack configuration.
#
# A real component would declare `aws_s3_bucket`, `aws_vpc`, and other taggable resources
# here. The AWS provider then applies every default tag to them automatically, so the
# component never needs a `tags` variable.

variable "name" {
  type        = string
  description = "Name of the resource this component manages."
}
