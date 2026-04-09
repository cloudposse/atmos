# VPC Component - for workdir provisioner testing

variable "vpc_name" {
  type        = string
  description = "Name of the VPC"
}

variable "cidr_block" {
  type        = string
  default     = "10.0.0.0/16"
  description = "CIDR block for the VPC"
}

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "enable_dns_hostnames" {
  type        = bool
  default     = true
  description = "Enable DNS hostnames in the VPC"
}

output "vpc_name" {
  value       = var.vpc_name
  description = "Name of the VPC"
}

output "cidr_block" {
  value       = var.cidr_block
  description = "CIDR block of the VPC"
}

output "environment" {
  value       = var.environment
  description = "Environment name"
}
