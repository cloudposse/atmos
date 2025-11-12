variable "name" {
  type        = string
  description = "VPC name"
}

variable "region" {
  type        = string
  description = "AWS region"
}

output "vpc_name" {
  value       = var.name
  description = "VPC name"
}
