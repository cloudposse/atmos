output "vpc_name" {
  description = "The VPC name."
  value       = var.name
}

output "vpc_cidr_block" {
  description = "The primary IPv4 CIDR block of the VPC."
  value       = var.ipv4_primary_cidr_block
}

output "availability_zones" {
  description = "The Availability Zones in which subnets are provisioned."
  value       = var.availability_zones
}

output "subnet_cidrs" {
  description = "The derived subnet CIDR blocks, one per Availability Zone."
  value       = local.subnet_cidrs
}

output "map_public_ip_on_launch" {
  description = "Whether instances receive a public IP address on launch."
  value       = var.map_public_ip_on_launch
}
