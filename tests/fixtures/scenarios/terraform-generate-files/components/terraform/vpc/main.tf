# VPC Component - for terraform generate files testing

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

variable "region" {
  type        = string
  default     = "us-east-1"
  description = "AWS region"
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
