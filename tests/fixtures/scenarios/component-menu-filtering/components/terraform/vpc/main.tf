# VPC Component - Mock for testing

variable "vpc_cidr" {
  type        = string
  default     = "10.0.0.0/16"
  description = "CIDR block for the VPC"
}

variable "environment" {
  type        = string
  default     = "dev"
  description = "Environment name"
}

variable "enable_dns_support" {
  type        = bool
  default     = true
  description = "Enable DNS support in VPC"
}

output "vpc_cidr" {
  value       = var.vpc_cidr
  description = "The VPC CIDR block"
}

output "environment" {
  value       = var.environment
  description = "The environment name"
}
