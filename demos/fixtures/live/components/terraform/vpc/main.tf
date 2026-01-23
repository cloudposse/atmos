# VPC Component for Live Demo
# Minimal terraform that can run plan without cloud credentials

variable "enabled" {
  description = "Whether to create resources"
  type        = bool
  default     = true
}

variable "name" {
  description = "Name of the VPC"
  type        = string
  default     = "vpc"
}

variable "cidr_block" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "dev"
}

variable "tags" {
  description = "Tags to apply"
  type        = map(string)
  default     = {}
}

# Context variables passed by atmos
variable "namespace" {
  description = "Namespace (organization)"
  type        = string
  default     = ""
}

variable "tenant" {
  description = "Tenant name"
  type        = string
  default     = ""
}

variable "stage" {
  description = "Stage (e.g., dev, staging, prod)"
  type        = string
  default     = ""
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = ""
}

variable "attributes" {
  description = "Additional attributes"
  type        = list(string)
  default     = []
}

# Local-only resource for demo - no cloud provider needed
resource "null_resource" "vpc" {
  count = var.enabled ? 1 : 0

  triggers = {
    name        = var.name
    cidr        = var.cidr_block
    environment = var.environment
  }
}

output "vpc_id" {
  description = "The ID of the VPC"
  value       = var.enabled ? "vpc-${var.name}-${var.environment}" : null
}

output "cidr_block" {
  description = "The CIDR block of the VPC"
  value       = var.enabled ? var.cidr_block : null
}

output "vpc_arn" {
  description = "The ARN of the VPC"
  value       = var.enabled ? "arn:aws:ec2:${var.region}:123456789012:vpc/vpc-${var.name}-${var.environment}" : null
}

output "private_subnet_ids" {
  description = "List of private subnet IDs"
  value       = var.enabled ? ["subnet-priv-a", "subnet-priv-b", "subnet-priv-c"] : []
}

output "public_subnet_ids" {
  description = "List of public subnet IDs"
  value       = var.enabled ? ["subnet-pub-a", "subnet-pub-b", "subnet-pub-c"] : []
}
