# Mock component for testing components without source

variable "enabled" {
  type        = bool
  description = "Set to false to prevent the module from creating any resources"
  default     = true
}

variable "environment" {
  type        = string
  description = "Environment name"
  default     = "dev"
}

output "enabled" {
  value       = var.enabled
  description = "Whether the module is enabled"
}

output "environment" {
  value       = var.environment
  description = "The environment name"
}
