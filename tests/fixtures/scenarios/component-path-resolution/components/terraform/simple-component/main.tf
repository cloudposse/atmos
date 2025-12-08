# Simple component for testing basic path resolution
# This component tests resolution when the component name matches the folder name directly

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
    component   = "simple-component"
    environment = var.environment
    enabled     = var.enabled
  }
}
