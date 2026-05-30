output "vpc_cidr" {
  description = "The CIDR block of the VPC"
  value       = var.cidr_block
}

output "environment" {
  description = "The environment name"
  value       = var.environment
}
