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

output "subnets" {
  value = {
    public : {
      ids : module.subnets.public_subnet_ids
      cidr : module.subnets.public_subnet_cidrs
    }
    private : {
      ids : module.subnets.private_subnet_ids
      cidr : module.subnets.private_subnet_cidrs
    }
  }
  description = "Subnets info map"
}

output "vpc_default_network_acl_id" {
  value       = module.vpc.vpc_default_network_acl_id
  description = "The ID of the network ACL created by default on VPC creation"
}

output "vpc_default_security_group_id" {
  value       = module.vpc.vpc_default_security_group_id
  description = "The ID of the security group created by default on VPC creation"
}

output "vpc_id" {
  value       = module.vpc.vpc_id
  description = "VPC ID"
}

output "vpc_cidr" {
  value       = module.vpc.vpc_cidr_block
  description = "VPC CIDR"
}

output "vpc" {
  value = {
    id : module.vpc.vpc_id
    cidr : module.vpc.vpc_cidr_block
    subnet_type_tag_key : var.subnet_type_tag_key
    # subnet_type_tag_value_format : var.subnet_type_tag_value_format
  }
  description = "VPC info map"
}

output "private_route_table_ids" {
  value       = module.subnets.private_route_table_ids
  description = "Private subnet route table IDs"
}

output "public_route_table_ids" {
  value       = module.subnets.public_route_table_ids
  description = "Public subnet route table IDs"
}

output "route_tables" {
  value = {
    public : {
      ids : module.subnets.public_route_table_ids
    }
    private : {
      ids : module.subnets.private_route_table_ids
    }
  }
  description = "Route tables info map"
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

output "nat_eip_protections" {
  description = "List of AWS Shield Advanced Protections for NAT Elastic IPs."
  value       = aws_shield_protection.nat_eip_shield_protection
}

output "interface_vpc_endpoints" {
  description = "List of Interface VPC Endpoints in this VPC."
  value       = try(module.vpc_endpoints[0].interface_vpc_endpoints, [])
}

output "availability_zones" {
  description = "List of Availability Zones where subnets were created"
  value       = module.subnets.availability_zones
}

output "az_private_subnets_map" {
  description = "Map of AZ names to list of private subnet IDs in the AZs"
  value       = module.subnets.az_private_subnets_map
}

output "az_public_subnets_map" {
  description = "Map of AZ names to list of public subnet IDs in the AZs"
  value       = module.subnets.az_public_subnets_map
}
