locals {
  az_set  = toset(var.availability_zones)
  az_list = tolist(local.az_set)
}


module "node_group" {
  for_each = module.this.enabled ? local.az_set : []

  source            = "../node_group_by_az"
  availability_zone = each.value

  node_group_size = {
    desired_size = floor((var.node_group_size.desired_size + index(local.az_list, each.value)) / length(local.az_list))
    min_size     = floor((var.node_group_size.min_size + index(local.az_list, each.value)) / length(local.az_list))
    max_size     = floor((var.node_group_size.max_size + index(local.az_list, each.value)) / length(local.az_list))
  }

  cluster_context = var.cluster_context
  context         = module.this.context
}
