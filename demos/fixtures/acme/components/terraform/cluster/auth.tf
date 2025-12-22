
locals {
  # Extract the cluster certificate for use in OIDC configuration
  certificate_authority_data = try(aws_eks_cluster.default[0].certificate_authority[0]["data"], "")

  eks_policy_short_abbreviation_map = {
    # List available policies with `aws eks list-access-policies --output table`

    Admin        = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSAdminPolicy"
    ClusterAdmin = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
    Edit         = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSEditPolicy"
    View         = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSViewPolicy"
    # Add new policies here
  }

  eks_policy_abbreviation_map = merge({ for k, v in local.eks_policy_short_abbreviation_map : format("AmazonEKS%sPolicy", k) => v },
  local.eks_policy_short_abbreviation_map)


  # Expand abbreviated access policies to full ARNs
  access_entry_expanded_map = { for k, v in var.access_entry_map : k => merge({
    # Expand abbreviated policies to full ARNs
    access_policy_associations = { for kk, vv in v.access_policy_associations : try(local.eks_policy_abbreviation_map[kk], kk) => vv }
    # Copy over all other fields
    }, { for kk, vv in v : kk => vv if kk != "access_policy_associations" })
  }

  # Replace membership in "system:masters" group with association to "ClusterAdmin" policy
  access_entry_map = { for k, v in local.access_entry_expanded_map : k => merge({
    # Remove "system:masters" group from standard users
    kubernetes_groups = [for group in v.kubernetes_groups : group if group != "system:masters" || v.type != "STANDARD"]
    access_policy_associations = merge(
      # copy all existing associations
      v.access_policy_associations,
      # add "ClusterAdmin" policy if the user was in "system:masters" group and is a standard user
      contains(v.kubernetes_groups, "system:masters") && v.type == "STANDARD" ? {
        "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy" = {
          access_scope = {
            type       = "cluster"
            namespaces = null
          }
        }
      } : {}
    )
    # Copy over all other fields
    }, { for kk, vv in v : kk => vv if kk != "kubernetes_groups" && kk != "access_policy_associations" })
  }

  eks_access_policy_association_product_map = merge(flatten([
    for k, v in local.access_entry_map : [for kk, vv in v.access_policy_associations : { format("%s-%s", k, kk) = {
      principal_arn = k
      policy_arn    = kk
      }
    }]
  ])...)
}

# The preferred way to keep track of entries is by key, but we also support list,
# because keys need to be known at plan time, but list values do not.
resource "aws_eks_access_entry" "map" {
  for_each = local.enabled ? local.access_entry_map : {}

  cluster_name      = local.eks_cluster_id
  principal_arn     = each.key
  kubernetes_groups = each.value.kubernetes_groups
  type              = each.value.type

  tags = module.this.tags
}

resource "aws_eks_access_policy_association" "map" {
  for_each = local.enabled ? local.eks_access_policy_association_product_map : {}

  cluster_name  = local.eks_cluster_id
  principal_arn = each.value.principal_arn
  policy_arn    = each.value.policy_arn

  access_scope {
    type       = local.access_entry_map[each.value.principal_arn].access_policy_associations[each.value.policy_arn].access_scope.type
    namespaces = local.access_entry_map[each.value.principal_arn].access_policy_associations[each.value.policy_arn].access_scope.namespaces
  }

  depends_on = [
    aws_eks_access_entry.map,
    aws_eks_access_entry.standard,
    aws_eks_access_entry.linux,
    aws_eks_access_entry.windows,
  ]
}

# We could combine all the list access entries into a single resource,
# but separating them by category minimizes the ripple effect of changes
# due to adding and removing items from the list.
resource "aws_eks_access_entry" "standard" {
  count = local.enabled ? length(var.access_entries) : 0

  cluster_name      = local.eks_cluster_id
  principal_arn     = var.access_entries[count.index].principal_arn
  kubernetes_groups = var.access_entries[count.index].kubernetes_groups
  type              = "STANDARD"

  tags = module.this.tags
}

resource "aws_eks_access_entry" "linux" {
  count = local.enabled ? length(lookup(var.access_entries_for_nodes, "EC2_LINUX", [])) : 0

  cluster_name  = local.eks_cluster_id
  principal_arn = var.access_entries_for_nodes.EC2_LINUX[count.index]
  type          = "EC2_LINUX"

  tags = module.this.tags
}

resource "aws_eks_access_entry" "windows" {
  count = local.enabled ? length(lookup(var.access_entries_for_nodes, "EC2_WINDOWS", [])) : 0

  cluster_name  = local.eks_cluster_id
  principal_arn = var.access_entries_for_nodes.EC2_WINDOWS[count.index]
  type          = "EC2_WINDOWS"

  tags = module.this.tags
}

resource "aws_eks_access_policy_association" "list" {
  count = local.enabled ? length(var.access_policy_associations) : 0

  cluster_name  = local.eks_cluster_id
  principal_arn = var.access_policy_associations[count.index].principal_arn
  policy_arn = try(local.eks_policy_abbreviation_map[var.access_policy_associations[count.index].policy_arn],
  var.access_policy_associations[count.index].policy_arn)

  access_scope {
    type       = var.access_policy_associations[count.index].access_scope.type
    namespaces = var.access_policy_associations[count.index].access_scope.namespaces
  }

  depends_on = [
    aws_eks_access_entry.map,
    aws_eks_access_entry.standard,
    aws_eks_access_entry.linux,
    aws_eks_access_entry.windows,
  ]
}
