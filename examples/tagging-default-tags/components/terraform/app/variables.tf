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

# Atmos passes the stack's context variables to every component. They name the stack, and the
# `terraform.providers` section reads them through templates, so the component only declares them.
variable "namespace" {
  type        = string
  description = "Organization identifier."
  default     = null
}

variable "tenant" {
  type        = string
  description = "Team or business unit that owns the resource."
  default     = null
}

variable "stage" {
  type        = string
  description = "Stage, such as dev or prod."
  default     = null
}

variable "region" {
  type        = string
  description = "AWS region."
  default     = null
}
