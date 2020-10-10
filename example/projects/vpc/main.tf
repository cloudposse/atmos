locals {
  # The usage of the specific kubernetes.io/cluster/* resource tags below are required
  # for EKS and Kubernetes to discover and manage networking resources
  # https://www.terraform.io/docs/providers/aws/guides/eks-getting-started.html#base-vpc-networking
  tags = map(format("kubernetes.io/cluster/%s-%s-%s-eks-cluster", var.namespace, var.environment, var.stage), "shared")

  availability_zones = length(var.availability_zones) > 0 ? var.availability_zones : var.region_availability_zones
  max_subnet_count = (
    var.max_subnet_count > 0 ? var.max_subnet_count : (
      length(var.region_availability_zones) > 0 ? length(var.region_availability_zones) : length(var.availability_zones)
    )
  )
}

module "vpc" {
  source = "git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=tags/0.17.0"

  tags       = local.tags
  cidr_block = var.cidr_block

  context = module.this.context
}

# https://docs.aws.amazon.com/eks/latest/userguide/network_reqs.html
locals {
  public_subnets_additional_tags = {
    "kubernetes.io/role/elb" : 1
  }

  private_subnets_additional_tags = {
    "kubernetes.io/role/internal-elb" : 1
  }
}

module "subnets" {
  source = "git::https://github.com/cloudposse/terraform-aws-dynamic-subnets.git?ref=tags/0.30.0"

  tags = local.tags

  availability_zones              = local.availability_zones
  cidr_block                      = module.vpc.vpc_cidr_block
  igw_id                          = module.vpc.igw_id
  map_public_ip_on_launch         = var.map_public_ip_on_launch
  max_subnet_count                = local.max_subnet_count
  nat_gateway_enabled             = var.nat_gateway_enabled
  nat_instance_enabled            = var.nat_instance_enabled
  nat_instance_type               = var.nat_instance_type
  public_subnets_additional_tags  = local.public_subnets_additional_tags
  private_subnets_additional_tags = local.private_subnets_additional_tags
  subnet_type_tag_key             = var.subnet_type_tag_key
  subnet_type_tag_value_format    = var.subnet_type_tag_value_format
  vpc_id                          = module.vpc.vpc_id

  context = module.this.context
}
