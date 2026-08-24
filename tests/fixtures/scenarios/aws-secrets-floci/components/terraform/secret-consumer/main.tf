terraform {
  required_version = ">= 1.3.0"
}

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
