# Simple terraform component for testing CLI flags
# This component uses local backend to avoid needing cloud credentials

terraform {
  backend "local" {}
}

variable "environment" {
  type        = string
  description = "Environment name"
  default     = "test"
}

variable "enabled" {
  type        = bool
  description = "Whether the component is enabled"
  default     = true
}

output "environment" {
  value       = var.environment
  description = "The environment name"
}

output "enabled" {
  value       = var.enabled
  description = "Whether the component is enabled"
}

output "message" {
  value       = "Hello from ${var.environment}"
  description = "A test message"
}
