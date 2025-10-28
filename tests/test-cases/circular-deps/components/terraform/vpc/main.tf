# Dummy VPC component for testing circular dependency detection.

variable "name" {
  type        = string
  description = "VPC name"
}

variable "cidr_block" {
  type        = string
  description = "CIDR block for VPC"
}

variable "transit_gateway_id" {
  type        = string
  description = "Transit Gateway ID"
  default     = null
}

variable "transit_gateway_attachments" {
  type        = any
  description = "Transit Gateway attachments"
  default     = null
}

output "vpc_id" {
  value       = "vpc-12345"
  description = "VPC ID"
}

output "transit_gateway_id" {
  value       = var.transit_gateway_id != null ? var.transit_gateway_id : "tgw-12345"
  description = "Transit Gateway ID"
}

output "attachment_ids" {
  value       = ["tgw-attach-12345"]
  description = "Transit Gateway attachment IDs"
}
