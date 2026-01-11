# Minimal terraform component for provenance testing.
variable "enabled" {
  type        = bool
  description = "Whether the app is enabled"
  default     = true
}

variable "name" {
  type        = string
  description = "Name of the app"
}

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "region" {
  type        = string
  description = "AWS region"
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply"
  default     = {}
}

output "app_name" {
  value = var.name
}
