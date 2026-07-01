terraform {
  required_version = ">= 1.3.0"
}

# This component has no resources. It exists to demonstrate that secrets stored
# in SSM / Secrets Manager resolve into Terraform variables via the `!secret`
# YAML function. Applying it validates the secret-resolution path end to end.

variable "ssm_instance_token" {
  type      = string
  sensitive = true
}

variable "ssm_stack_token" {
  type      = string
  sensitive = true
}

variable "asm_database_password" {
  type      = string
  sensitive = true
}

variable "global_shared_token" {
  type      = string
  sensitive = true
}
