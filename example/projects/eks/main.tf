data "terraform_remote_state" "vpc" {
  backend = "local"
  config = {
    path = "${path.module}/../vpc/${terraform.workspace}-terraform.tfstate"
  }
}

locals {
  vpc_outputs        = data.terraform_remote_state.vpc.outputs
  public_subnet_ids  = local.vpc_outputs.public_subnet_ids
  private_subnet_ids = local.vpc_outputs.private_subnet_ids
  vpc_id             = local.vpc_outputs.vpc_id
}

module "eks_cluster" {
  source = "git::https://github.com/cloudposse/terraform-aws-eks-cluster.git?ref=tags/0.28.0"

  region     = var.region
  attributes = module.this.attributes

  allowed_security_groups      = var.allowed_security_groups
  allowed_cidr_blocks          = var.allowed_cidr_blocks
  apply_config_map_aws_auth    = var.apply_config_map_aws_auth
  cluster_log_retention_period = var.cluster_log_retention_period
  enabled_cluster_log_types    = var.enabled_cluster_log_types
  endpoint_private_access      = var.cluster_endpoint_private_access
  endpoint_public_access       = var.cluster_endpoint_public_access
  kubernetes_version           = var.cluster_kubernetes_version
  oidc_provider_enabled        = var.oidc_provider_enabled
  map_additional_aws_accounts  = var.map_additional_aws_accounts
  map_additional_iam_roles     = []
  map_additional_iam_users     = var.map_additional_iam_users
  public_access_cidrs          = var.public_access_cidrs
  subnet_ids                   = concat(local.private_subnet_ids, local.public_subnet_ids)
  vpc_id                       = local.vpc_id

  kubernetes_config_map_ignore_role_changes = false
  workers_role_arns                         = []

  context = module.this.context
}

locals {
  node_group_default_availability_zones = var.node_group_defaults.availability_zones == null ? var.region_availability_zones : var.node_group_defaults.availability_zones
  node_group_default_kubernetes_version = var.node_group_defaults.kubernetes_version == null ? var.cluster_kubernetes_version : var.node_group_defaults.kubernetes_version

  node_groups          = flatten([for m in values(module.region_node_group)[*].region_node_groups : values(m)])
  node_group_arns      = compact([for group in local.node_groups : group.eks_node_group_arn])
  node_group_role_arns = compact([for group in local.node_groups : group.eks_node_group_role_arn])
}

module "region_node_group" {
  for_each = module.this.enabled ? var.node_groups : {}

  source = "./modules/node_group_by_region"

  availability_zones = each.value.availability_zones == null ? local.node_group_default_availability_zones : each.value.availability_zones
  attributes         = flatten(concat(var.attributes, [each.key], each.value.attributes == null ? var.node_group_defaults.attributes : each.value.attributes))

  node_group_size = module.this.enabled ? {
    desired_size = each.value.desired_group_size == null ? var.node_group_defaults.desired_group_size : each.value.desired_group_size
    min_size     = each.value.min_group_size == null ? var.node_group_defaults.min_group_size : each.value.min_group_size
    max_size     = each.value.max_group_size == null ? var.node_group_defaults.max_group_size : each.value.max_group_size
  } : null

  cluster_context = module.this.enabled ? {
    cluster_name              = module.eks_cluster.eks_cluster_id
    create_before_destroy     = each.value.create_before_destroy == null ? var.node_group_defaults.create_before_destroy : each.value.create_before_destroy
    disk_size                 = each.value.disk_size == null ? var.node_group_defaults.disk_size : each.value.disk_size
    enable_cluster_autoscaler = each.value.enable_cluster_autoscaler == null ? var.node_group_defaults.enable_cluster_autoscaler : each.value.enable_cluster_autoscaler
    instance_types            = each.value.instance_types == null ? var.node_group_defaults.instance_types : each.value.instance_types
    ami_type                  = each.value.ami_type == null ? var.node_group_defaults.ami_type : each.value.ami_type
    ami_release_version       = each.value.ami_release_version == null ? var.node_group_defaults.ami_release_version : each.value.ami_release_version
    kubernetes_version        = each.value.kubernetes_version == null ? local.node_group_default_kubernetes_version : each.value.kubernetes_version
    kubernetes_labels         = each.value.kubernetes_labels == null ? var.node_group_defaults.kubernetes_labels : each.value.kubernetes_labels
    kubernetes_taints         = each.value.kubernetes_taints == null ? var.node_group_defaults.kubernetes_taints : each.value.kubernetes_taints
    resources_to_tag          = each.value.resources_to_tag == null ? var.node_group_defaults.resources_to_tag : each.value.resources_to_tag
    subnet_type_tag_key       = var.subnet_type_tag_key
    vpc_id                    = local.vpc_id

    module_depends_on = module.eks_cluster.kubernetes_config_map_id
  } : null

  context = module.this.context
}
