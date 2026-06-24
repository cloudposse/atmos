# Mock component for locals demo.

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

variable "deploy_target" {
  type        = string
  description = "Deployment target (computed from conditional locals)"
  default     = "stable"
}

variable "managed_by" {
  type        = string
  description = "Tool that manages this resource"
  default     = "atmos"
}

output "name" {
  value = var.name
}

output "full_name" {
  value = var.full_name
}

output "deploy_target" {
  value = var.deploy_target
}

output "tags" {
  value = var.tags
}
