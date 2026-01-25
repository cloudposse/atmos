locals {
  account_map_enabled = local.enabled && var.account_map_enabled
}

module "eks" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.8.0"

  bypass = !local.account_map_enabled

  component = var.eks_component_name

  context = module.this.context

  defaults = {
    eks_cluster_id                         = coalesce(var.eks.eks_cluster_id, "deleted")
    eks_cluster_arn                        = coalesce(var.eks.eks_cluster_arn, "deleted")
    eks_cluster_endpoint                   = coalesce(var.eks.eks_cluster_endpoint, "deleted")
    eks_cluster_certificate_authority_data = var.eks.eks_cluster_certificate_authority_data
    eks_cluster_identity_oidc_issuer       = coalesce(var.eks.eks_cluster_identity_oidc_issuer, "deleted")
    karpenter_iam_role_arn                 = coalesce(var.eks.karpenter_iam_role_arn, "deleted")
  }
}
