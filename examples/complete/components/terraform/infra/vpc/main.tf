locals {
  # For EKS and Kubernetes to discover and manage networking resources
  # https://www.terraform.io/docs/providers/aws/guides/eks-getting-started.html#base-vpc-networking
  tags = {
    format("kubernetes.io/cluster/%s-%s-%s-eks-cluster", module.this.namespace, module.this.environment, module.this.stage) = "shared"
  }

  availability_zones = length(var.availability_zones) > 0 ? var.availability_zones : var.region_availability_zones

  max_subnet_count = (
    var.max_subnet_count > 0 ? var.max_subnet_count : (
      length(var.region_availability_zones) > 0 ? length(var.region_availability_zones) : length(var.availability_zones)
    )
  )

  # Required tags to make ALB ingress work https://docs.aws.amazon.com/eks/latest/userguide/alb-ingress.html
  # https://docs.aws.amazon.com/eks/latest/userguide/network_reqs.html
  public_subnets_additional_tags = {
    "kubernetes.io/role/elb" : 1
  }

  private_subnets_additional_tags = {
    "kubernetes.io/role/internal-elb" : 1
  }
}

module "vpc" {
  source  = "cloudposse/vpc/aws"
  version = "1.1.1"

  ipv4_primary_cidr_block         = var.cidr_block
  instance_tenancy                = var.instance_tenancy
  dns_hostnames_enabled           = var.dns_hostnames_enabled
  dns_support_enabled             = var.dns_support_enabled
  classiclink_enabled             = var.classiclink_enabled
  classiclink_dns_support_enabled = var.classiclink_dns_support_enabled

  tags = local.tags

  context = module.this.context
}

module "subnets" {
  source  = "cloudposse/dynamic-subnets/aws"
  version = "2.0.3"

  availability_zones              = local.availability_zones
  ipv4_cidr_block                 = [module.vpc.vpc_cidr_block]
  igw_id                          = [module.vpc.igw_id]
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

  tags = local.tags

  context = module.this.context
}
