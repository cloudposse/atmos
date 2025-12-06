# Dummy test component for testing circular dependency detection.

variable "name" {
  type        = string
  description = "Component name"
}

variable "dependency" {
  type        = any
  description = "Dependency value from another component"
  default     = null
}

variable "value" {
  type        = string
  description = "Test value"
  default     = "default"
}

output "value" {
  value       = var.value
  description = "Output value"
}
