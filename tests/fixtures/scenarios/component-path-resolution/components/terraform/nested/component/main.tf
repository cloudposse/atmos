# Nested component for testing nested path resolution
# This component tests resolution when the component is in a subfolder (nested/component)

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "enabled" {
  type        = bool
  description = "Whether the component is enabled"
  default     = true
}

output "component_info" {
  value = {
    component   = "nested/component"
    environment = var.environment
    enabled     = var.enabled
  }
}
