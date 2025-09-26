# Mock context.tf for testing vendor functionality
# This file simulates the terraform-null-label context.tf

variable "namespace" {
  type        = string
  default     = ""
  description = "ID element. Usually the organization name"
}

variable "environment" {
  type        = string
  default     = ""
  description = "ID element. Usually used for region e.g. 'uw2', 'us-west-2'"
}

variable "stage" {
  type        = string
  default     = ""
  description = "ID element. Usually used to indicate role, e.g. 'prod', 'staging', 'source', 'build', 'test', 'deploy', 'release'"
}

variable "name" {
  type        = string
  default     = ""
  description = "ID element. Usually the component name"
}
