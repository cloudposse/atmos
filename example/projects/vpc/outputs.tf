output "public_subnet_ids" {
  value       = module.subnets.public_subnet_ids
  description = "Public subnet IDs"
}

output "public_subnet_cidrs" {
  value       = module.subnets.public_subnet_cidrs
  description = "Public subnet CIDRs"
}

output "private_subnet_ids" {
  value       = module.subnets.private_subnet_ids
  description = "Private subnet IDs"
}

output "private_subnet_cidrs" {
  value       = module.subnets.private_subnet_cidrs
  description = "Private subnet CIDRs"
}

output "vpc_id" {
  value       = module.vpc.vpc_id
  description = "VPC ID"
}

output "vpc_cidr" {
  value       = module.vpc.vpc_cidr_block
  description = "VPC CIDR"
}

output "private_route_table_ids" {
  value       = module.subnets.private_route_table_ids
  description = "Private subnet route table IDs"
}

output "public_route_table_ids" {
  value       = module.subnets.public_route_table_ids
  description = "Public subnet route table IDs"
}

output "nat_gateway_ids" {
  value       = module.subnets.nat_gateway_ids
  description = "NAT Gateway IDs"
}

output "nat_instance_ids" {
  value       = module.subnets.nat_instance_ids
  description = "NAT Instance IDs"
}

output "nat_gateway_public_ips" {
  value       = module.subnets.nat_gateway_public_ips
  description = "NAT Gateway public IPs"
}

output "max_subnet_count" {
  value       = local.max_subnet_count
  description = "Maximum allowed number of subnets before all subnet CIDRs need to be recomputed"
}
