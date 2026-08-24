# Mock component for testing remote stack imports.
# This component doesn't provision any real resources.

variable "name" {
  type        = string
  description = "Name of the application"
  default     = "myapp"
}

variable "environment" {
  type        = string
  description = "Deployment environment"
  default     = "dev"
}

variable "imported_from" {
  type        = string
  description = "Source of the import (local or remote)"
  default     = "unknown"
}

variable "remote_import_test" {
  type        = bool
  description = "Flag to indicate this was imported from remote"
  default     = false
}

variable "shared_setting" {
  type        = string
  description = "A shared setting imported from remote"
  default     = ""
}

output "name" {
  value       = var.name
  description = "Application name"
}

output "environment" {
  value       = var.environment
  description = "Deployment environment"
}

output "imported_from" {
  value       = var.imported_from
  description = "Import source"
}

output "remote_import_test" {
  value       = var.remote_import_test
  description = "Whether this was imported from remote"
}

output "shared_setting" {
  value       = var.shared_setting
  description = "Shared setting from remote import"
}
