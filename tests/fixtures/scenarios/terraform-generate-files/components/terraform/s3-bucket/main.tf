# S3 Bucket Component - for terraform generate files testing

variable "bucket_name" {
  type        = string
  description = "Name of the S3 bucket"
}

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "versioning_enabled" {
  type        = bool
  default     = false
  description = "Enable versioning on the bucket"
}

output "bucket_name" {
  value       = var.bucket_name
  description = "Name of the S3 bucket"
}

output "environment" {
  value       = var.environment
  description = "Environment name"
}

output "versioning_enabled" {
  value       = var.versioning_enabled
  description = "Versioning status"
}
