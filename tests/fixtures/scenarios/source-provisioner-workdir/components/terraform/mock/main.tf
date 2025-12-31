# Mock Terraform component for testing
# This is a local component used for comparison with remote-sourced components

variable "enabled" {
  type        = bool
  default     = true
  description = "Whether the component is enabled"
}

variable "environment" {
  type        = string
  default     = "test"
  description = "Environment name"
}

output "component_type" {
  value       = "local-mock"
  description = "Indicates this is a local mock component"
}

output "environment" {
  value       = var.environment
  description = "The configured environment"
}
