locals {
  account_map_enabled = local.enabled && var.account_map_enabled
}

module "eks" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.8.0"

  bypass    = !local.account_map_enabled
  component = var.eks_component_name

  defaults = {
    eks_cluster_id                         = coalesce(var.eks.eks_cluster_id, "deleted")
    eks_cluster_arn                        = coalesce(var.eks.eks_cluster_arn, "deleted")
    eks_cluster_endpoint                   = var.eks.eks_cluster_endpoint
    eks_cluster_certificate_authority_data = var.eks.eks_cluster_certificate_authority_data
    eks_cluster_identity_oidc_issuer       = coalesce(var.eks.eks_cluster_identity_oidc_issuer, "deleted")
    karpenter_iam_role_name                = var.eks.karpenter_iam_role_name
    karpenter_node_role_arn                = coalesce(var.eks.karpenter_node_role_arn, "deleted")
  }

  context = module.this.context
}

module "vpc" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.8.0"

  bypass    = !local.account_map_enabled
  component = var.vpc_component_name

  defaults = {
    private_subnet_ids = var.vpc.private_subnet_ids
    public_subnet_ids  = var.vpc.public_subnet_ids
  }

  context = module.this.context
}
