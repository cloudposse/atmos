# VPC Component Outputs

output "vpc_cidr" {
  description = "CIDR block of the VPC"
  value       = var.vpc_cidr
}

output "availability_zones" {
  description = "Availability zones used"
  value       = var.availability_zones
}

output "nat_gateway_enabled" {
  description = "Whether NAT Gateways are enabled"
  value       = var.nat_gateway_enabled
}

output "subnet_count" {
  description = "Number of subnets per type"
  value       = local.subnet_count
}

output "tags" {
  description = "Resource tags"
  value       = var.tags
}
