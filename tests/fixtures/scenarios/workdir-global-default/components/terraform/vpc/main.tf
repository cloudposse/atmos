# VPC Component - for global workdir default testing

variable "vpc_name" {
  type        = string
  description = "Name of the VPC"
}

output "vpc_name" {
  value       = var.vpc_name
  description = "Name of the VPC"
}
