locals {
  enabled = module.this.enabled

  nat_eip_aws_shield_protection_enabled = local.enabled && var.nat_eip_aws_shield_protection_enabled
  vpc_flow_logs_enabled                 = local.enabled && var.vpc_flow_logs_enabled

  # The usage of specific kubernetes.io/cluster/* resource tags were required before Kubernetes 1.19,
  # but are now deprecated. See https://docs.aws.amazon.com/eks/latest/userguide/network_reqs.html

  max_subnet_count = (
    var.max_subnet_count > 0 ? var.max_subnet_count : (
      length(var.availability_zone_ids) > 0 ? length(var.availability_zone_ids) : length(var.availability_zones)
    )
  )

  availability_zones = length(var.availability_zones) > 0 ? (
    (substr(
      var.availability_zones[0],
      0,
      length(var.region)
    ) == var.region) ? var.availability_zones : formatlist("${var.region}%s", var.availability_zones)
  ) : var.availability_zones

  short_region = module.utils.region_az_alt_code_maps["to_short"][var.region]

  availability_zone_ids = length(var.availability_zone_ids) > 0 ? (
    (substr(
      var.availability_zone_ids[0],
      0,
      length(local.short_region)
    ) == local.short_region) ? var.availability_zone_ids : formatlist("${local.short_region}%s", var.availability_zone_ids)
  ) : var.availability_zone_ids

  # required tags to make ALB ingress work https://docs.aws.amazon.com/eks/latest/userguide/alb-ingress.html
  # https://docs.aws.amazon.com/eks/latest/userguide/network_reqs.html
  public_subnets_additional_tags = {
    (var.subnet_type_tag_key) = "public",
    "kubernetes.io/role/elb"  = 1,
  }

  private_subnets_additional_tags = {
    (var.subnet_type_tag_key)         = "private",
    "kubernetes.io/role/internal-elb" = 1,
  }

  gateway_endpoint_map = { for v in var.gateway_vpc_endpoints : v => {
    name            = v
    policy          = null
    route_table_ids = module.subnets.private_route_table_ids
  } }

  # If we use a separate security group for each endpoint interface,
  # we will use the interface service name as the key:
  #       security_group_ids  = [module.endpoint_security_groups[v].id]
  # If we use a single security group for all endpoint interfaces,
  # we will use local.interface_endpoint_security_group_key as the key.
  interface_endpoint_security_group_key = "VPC Endpoint interfaces"

  interface_endpoint_map = { for v in var.interface_vpc_endpoints : v => {
    name                = v
    policy              = null
    private_dns_enabled = true # Allow applications to use normal service DNS names to access the service
    security_group_ids  = [module.endpoint_security_groups[local.interface_endpoint_security_group_key].id]
    subnet_ids          = module.subnets.private_subnet_ids
  } }
}

module "utils" {
  source  = "cloudposse/utils/aws"
  version = "1.3.0"
}

module "vpc" {
  source  = "cloudposse/vpc/aws"
  version = "2.1.0"

  ipv4_primary_cidr_block          = var.ipv4_primary_cidr_block
  internet_gateway_enabled         = var.public_subnets_enabled
  assign_generated_ipv6_cidr_block = var.assign_generated_ipv6_cidr_block

  ipv4_primary_cidr_block_association     = var.ipv4_primary_cidr_block_association
  ipv4_additional_cidr_block_associations = var.ipv4_additional_cidr_block_associations
  ipv4_cidr_block_association_timeouts    = var.ipv4_cidr_block_association_timeouts

  # Required for DNS resolution of VPC Endpoint interfaces, and generally harmless
  # See https://docs.aws.amazon.com/vpc/latest/userguide/vpc-dns.html#vpc-dns-support
  dns_hostnames_enabled = true
  dns_support_enabled   = true

  context = module.this.context
}

# We could create a security group per endpoint,
# but until we are ready to customize them by service, it is just a waste
# of resources. We use a single security group for all endpoints.
# Security groups can be updated without recreating the endpoint or
# interrupting service, so this is an easy change to make later.
module "endpoint_security_groups" {
  for_each = local.enabled && try(length(var.interface_vpc_endpoints), 0) > 0 ? toset([local.interface_endpoint_security_group_key]) : []

  source  = "cloudposse/security-group/aws"
  version = "2.2.0"

  create_before_destroy      = true
  preserve_security_group_id = false
  attributes                 = [each.value]
  vpc_id                     = module.vpc.vpc_id

  rules_map = {
    ingress = [{
      key              = "vpc_ingress"
      type             = "ingress"
      from_port        = 0
      to_port          = 65535
      protocol         = "-1" # allow ping
      cidr_blocks      = compact(concat([module.vpc.vpc_cidr_block], module.vpc.additional_cidr_blocks))
      ipv6_cidr_blocks = compact(concat([module.vpc.vpc_ipv6_cidr_block], module.vpc.additional_ipv6_cidr_blocks))
      description      = "Ingress from VPC to ${each.value}"
    }]
  }

  allow_all_egress = true

  context = module.this.context
}

module "vpc_endpoints" {
  source  = "cloudposse/vpc/aws//modules/vpc-endpoints"
  version = "2.1.0"

  enabled = local.enabled && (length(var.interface_vpc_endpoints) + length(var.gateway_vpc_endpoints)) > 0

  vpc_id                  = module.vpc.vpc_id
  gateway_vpc_endpoints   = local.gateway_endpoint_map
  interface_vpc_endpoints = local.interface_endpoint_map

  context = module.this.context
}

module "subnets" {
  source  = "cloudposse/dynamic-subnets/aws"
  version = "2.3.0"

  availability_zones              = local.availability_zones
  availability_zone_ids           = local.availability_zone_ids
  ipv4_cidr_block                 = [module.vpc.vpc_cidr_block]
  ipv4_cidrs                      = var.ipv4_cidrs
  ipv6_enabled                    = false
  igw_id                          = var.public_subnets_enabled ? [module.vpc.igw_id] : []
  map_public_ip_on_launch         = var.map_public_ip_on_launch
  max_subnet_count                = local.max_subnet_count
  nat_gateway_enabled             = var.nat_gateway_enabled
  nat_instance_enabled            = var.nat_instance_enabled
  nat_instance_type               = var.nat_instance_type
  nat_instance_ami_id             = var.nat_instance_ami_id
  public_subnets_enabled          = var.public_subnets_enabled
  public_subnets_additional_tags  = local.public_subnets_additional_tags
  private_subnets_additional_tags = local.private_subnets_additional_tags
  vpc_id                          = module.vpc.vpc_id

  context = module.this.context
}

data "aws_caller_identity" "current" {
  count = local.nat_eip_aws_shield_protection_enabled ? 1 : 0
}

data "aws_eip" "eip" {
  for_each = local.nat_eip_aws_shield_protection_enabled ? toset(module.subnets.nat_ips) : []

  public_ip = each.key
}

resource "aws_shield_protection" "nat_eip_shield_protection" {
  for_each = local.nat_eip_aws_shield_protection_enabled ? data.aws_eip.eip : {}

  name         = data.aws_eip.eip[each.key].id
  resource_arn = "arn:aws:ec2:${var.region}:${data.aws_caller_identity.current[0].account_id}:eip-allocation/${data.aws_eip.eip[each.key].id}"
}
