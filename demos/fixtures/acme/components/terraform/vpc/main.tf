locals {
  enabled = module.this.enabled

  default_adoption_tags = merge(module.label.tags, {
    Name = join(module.label.delimiter, [module.label.id, "default"])
  })

  # Local variables corresponding to v2 module inputs that replace v0 inputs
  ipv4_primary_cidr_block          = var.ipv4_primary_cidr_block
  ipv4_cidr_block_associations     = var.ipv4_additional_cidr_block_associations
  assign_generated_ipv6_cidr_block = var.assign_generated_ipv6_cidr_block
  dns_hostnames_enabled            = var.dns_hostnames_enabled
  dns_support_enabled              = var.dns_support_enabled
  default_security_group_deny_all  = local.enabled && var.default_security_group_deny_all
  internet_gateway_enabled         = local.enabled && var.internet_gateway_enabled

  # Local variables that should be false when the module is disabled
  default_route_table_no_routes             = local.enabled && var.default_route_table_no_routes
  default_network_acl_deny_all              = local.enabled && var.default_network_acl_deny_all
  ipv6_egress_only_internet_gateway_enabled = local.enabled && var.ipv6_egress_only_internet_gateway_enabled
}

module "label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  context = module.this.context
}

module "igw_label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  enabled    = local.internet_gateway_enabled
  attributes = ["igw"]
  context    = module.this.context
}

module "eigw_label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  enabled    = local.ipv6_egress_only_internet_gateway_enabled
  attributes = ["eigw"]
  context    = module.this.context
}

resource "aws_vpc" "default" {
  count = local.enabled ? 1 : 0

  #bridgecrew:skip=BC_AWS_LOGGING_9:VPC Flow Logs are meant to be enabled by terraform-aws-vpc-flow-logs-s3-bucket and/or terraform-aws-cloudwatch-flow-logs
  cidr_block          = local.ipv4_primary_cidr_block
  ipv4_ipam_pool_id   = try(var.ipv4_primary_cidr_block_association.ipv4_ipam_pool_id, null)
  ipv4_netmask_length = try(var.ipv4_primary_cidr_block_association.ipv4_netmask_length, null)
  # Additional IPv4 CIDRs are handled by aws_vpc_ipv4_cidr_block_association below

  ipv6_cidr_block     = try(var.ipv6_primary_cidr_block_association.ipv6_cidr_block, null)
  ipv6_ipam_pool_id   = try(var.ipv6_primary_cidr_block_association.ipv6_ipam_pool_id, null)
  ipv6_netmask_length = try(var.ipv6_primary_cidr_block_association.ipv6_netmask_length, null)
  # Additional IPv6 CIDRs are handled by aws_vpc_ipv6_cidr_block_association below

  instance_tenancy                     = var.instance_tenancy
  enable_dns_hostnames                 = local.dns_hostnames_enabled
  enable_dns_support                   = local.dns_support_enabled
  assign_generated_ipv6_cidr_block     = local.assign_generated_ipv6_cidr_block
  enable_network_address_usage_metrics = var.network_address_usage_metrics_enabled
  tags                                 = module.label.tags
}

# If `aws_default_security_group` is not defined, it will be created implicitly with access `0.0.0.0/0`
resource "aws_default_security_group" "default" {
  count = local.default_security_group_deny_all ? 1 : 0

  vpc_id = aws_vpc.default[0].id
  tags   = local.default_adoption_tags
}

# If `aws_default_route_table` is not defined, it will be created implicitly with default routes
resource "aws_default_route_table" "default" {
  count = local.default_route_table_no_routes ? 1 : 0

  default_route_table_id = aws_vpc.default[0].default_route_table_id
  tags                   = local.default_adoption_tags
}

# If `aws_default_network_acl` is not defined, it will be created implicitly with access `0.0.0.0/0`
resource "aws_default_network_acl" "default" {
  count = local.default_network_acl_deny_all ? 1 : 0

  default_network_acl_id = aws_vpc.default[0].default_network_acl_id
  tags                   = local.default_adoption_tags
}

resource "aws_internet_gateway" "default" {
  count = local.internet_gateway_enabled ? 1 : 0

  vpc_id = aws_vpc.default[0].id
  tags   = module.igw_label.tags
}

resource "aws_egress_only_internet_gateway" "default" {
  count = local.ipv6_egress_only_internet_gateway_enabled ? 1 : 0

  vpc_id = aws_vpc.default[0].id
  tags   = module.eigw_label.tags
}

resource "aws_vpc_ipv4_cidr_block_association" "default" {
  for_each = local.enabled ? local.ipv4_cidr_block_associations : {}

  cidr_block          = each.value.ipv4_cidr_block
  ipv4_ipam_pool_id   = each.value.ipv4_ipam_pool_id
  ipv4_netmask_length = each.value.ipv4_netmask_length

  vpc_id = aws_vpc.default[0].id

  dynamic "timeouts" {
    for_each = local.enabled && var.ipv4_cidr_block_association_timeouts != null ? [true] : []
    content {
      create = lookup(var.ipv4_cidr_block_association_timeouts, "create", null)
      delete = lookup(var.ipv4_cidr_block_association_timeouts, "delete", null)
    }
  }
}

resource "aws_vpc_ipv6_cidr_block_association" "default" {
  for_each = local.enabled ? var.ipv6_additional_cidr_block_associations : {}

  ipv6_cidr_block     = each.value.ipv6_cidr_block
  ipv6_ipam_pool_id   = each.value.ipv6_ipam_pool_id
  ipv6_netmask_length = each.value.ipv6_netmask_length

  vpc_id = aws_vpc.default[0].id

  dynamic "timeouts" {
    for_each = local.enabled && var.ipv6_cidr_block_association_timeouts != null ? [true] : []
    content {
      create = lookup(var.ipv6_cidr_block_association_timeouts, "create", null)
      delete = lookup(var.ipv6_cidr_block_association_timeouts, "delete", null)
    }
  }
}
