data "aws_subnet_ids" "private" {
  count = local.enabled ? 1 : 0

  vpc_id = var.cluster_context.vpc_id

  tags = {
    "${var.cluster_context.subnet_type_tag_key}" = "private"
  }

  filter {
    name   = "availability-zone"
    values = [var.availability_zone]
  }
}

locals {
  enabled         = module.this.enabled && length(var.availability_zone) > 0
  sentinel        = "~~"
  subnet_ids_test = coalescelist(flatten(data.aws_subnet_ids.private[*].ids), [local.sentinel])
  subnet_ids      = local.subnet_ids_test[0] == local.sentinel ? null : local.subnet_ids_test
  az_attribute    = replace(var.availability_zone, "/^((.)[^-]*-(.)[^-]*-)/", "$2$3")
}

module "eks_node_group" {
  source  = "git::https://github.com/cloudposse/terraform-aws-eks-node-group.git?ref=tags/0.12.0"
  enabled = local.enabled

  attributes = length(var.availability_zone) > 0 ? flatten([module.this.attributes, local.az_attribute]) : module.this.attributes

  desired_size = local.enabled ? var.node_group_size.desired_size : null
  min_size     = local.enabled ? var.node_group_size.min_size : null
  max_size     = local.enabled ? var.node_group_size.max_size : null

  cluster_name              = local.enabled ? var.cluster_context.cluster_name : null
  create_before_destroy     = local.enabled ? var.cluster_context.create_before_destroy : null
  disk_size                 = local.enabled ? var.cluster_context.disk_size : null
  enable_cluster_autoscaler = local.enabled ? var.cluster_context.enable_cluster_autoscaler : null
  instance_types            = local.enabled ? var.cluster_context.instance_types : null
  ami_type                  = local.enabled ? var.cluster_context.ami_type : null
  ami_release_version       = local.enabled ? var.cluster_context.ami_release_version : null
  kubernetes_labels         = local.enabled ? var.cluster_context.kubernetes_labels : null
  kubernetes_taints         = local.enabled ? var.cluster_context.kubernetes_taints : null
  kubernetes_version        = local.enabled ? var.cluster_context.kubernetes_version : null
  resources_to_tag          = local.enabled ? var.cluster_context.resources_to_tag : null
  subnet_ids                = local.enabled ? local.subnet_ids : null
  # Prevent the node groups from being created before the Kubernetes aws-auth configMap
  module_depends_on = var.cluster_context.module_depends_on

  context = module.this.context
}
