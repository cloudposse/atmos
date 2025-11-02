# Minimal Terraform component for AuthManager propagation testing.
# This component doesn't need to do anything - we're just testing that
# ExecuteDescribeComponent correctly extracts AuthContext from AuthManager.

variable "enabled" {
  type        = bool
  description = "Enable this component"
  default     = true
}

variable "name" {
  type        = string
  description = "Component name"
}

variable "environment" {
  type        = string
  description = "Environment name"
  default     = "test"
}

output "name" {
  value       = var.name
  description = "Component name"
}

output "environment" {
  value       = var.environment
  description = "Environment"
}
