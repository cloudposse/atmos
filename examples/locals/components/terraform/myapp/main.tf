# Mock component for locals demo.
#
# This component outputs the variables it receives, allowing you to see
# how locals are resolved and passed to components.

variable "name" {
  type        = string
  description = "Application name"
}

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "full_name" {
  type        = string
  description = "Full application name (computed from locals)"
}

variable "tags" {
  type        = map(string)
  description = "Resource tags (computed from locals)"
  default     = {}
}

output "name" {
  description = "Application name"
  value       = var.name
}

output "environment" {
  description = "Environment name"
  value       = var.environment
}

output "full_name" {
  description = "Full application name"
  value       = var.full_name
}

output "tags" {
  description = "Resource tags"
  value       = var.tags
}
