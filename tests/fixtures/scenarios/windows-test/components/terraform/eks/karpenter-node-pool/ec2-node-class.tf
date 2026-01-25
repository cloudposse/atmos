# This provisions the EC2NodeClass for the NodePool.
# https://karpenter.sh/docs/concepts/nodeclasses/
#
# We keep it separate from the NodePool creation,
# even though there is a 1-to-1 mapping between the two,
# to make it a little easier to compare the implementation here
# with the Karpenter documentation, and to track changes as
# Karpenter evolves.
#


locals {
  # If you include a field but set it to null, the field will be omitted from the Kubernetes resource,
  # but the Kubernetes provider will still try to include it with a null value,
  # which will cause perpetual diff in the Terraform plan.
  # We strip out the null values from block_device_mappings here, because it is too complicated to do inline.
  node_block_device_mappings = { for pk, pv in local.node_pools : pk => [
    for i, map in pv.block_device_mappings : merge({
      for dk, dv in map : dk => dv if dk != "ebs" && dv != null
    }, try(length(map.ebs), 0) == 0 ? {} : { ebs = { for ek, ev in map.ebs : ek => ev if ev != null } })
    ]
  }
}

# https://karpenter.sh/docs/concepts/nodeclasses/
resource "kubernetes_manifest" "ec2_node_class" {
  for_each = local.node_pools

  manifest = {
    apiVersion = "karpenter.k8s.aws/v1"
    kind       = "EC2NodeClass"
    metadata = {
      name = coalesce(each.value.name, each.key)
    }
    spec = merge({
      role = module.eks.outputs.karpenter_iam_role_name
      subnetSelectorTerms = [for id in(each.value.private_subnets_enabled ? local.private_subnet_ids : local.public_subnet_ids) : {
        id = id
      }]
      securityGroupSelectorTerms = [{
        tags = {
          "aws:eks:cluster-name" = local.eks_cluster_id
        }
      }]
      # https://karpenter.sh/v1.0/concepts/nodeclasses/#specamiselectorterms
      amiSelectorTerms   = each.value.ami_selector_terms
      metadataOptions    = each.value.metadata_options
      tags               = module.this.tags
      detailedMonitoring = each.value.detailed_monitoring
      userData           = each.value.user_data != null ? each.value.user_data : null
      }, try(length(local.node_block_device_mappings[each.key]), 0) == 0 ? {} : {
      blockDeviceMappings = local.node_block_device_mappings[each.key]
      },
      each.value.ami_family == null ? {} : {
        amiFamily = each.value.ami_family
    })
  }
}
