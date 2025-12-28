# S3 Bucket Component - for workdir provisioner testing

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
  default     = true
  description = "Enable versioning on the bucket"
}

variable "encryption_enabled" {
  type        = bool
  default     = true
  description = "Enable encryption on the bucket"
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
  description = "Whether versioning is enabled"
}
