locals {
  enabled = module.this.enabled

  use_ipv6 = var.kubernetes_network_ipv6_enabled

  eks_cluster_id = one(aws_eks_cluster.default[*].id)

  cluster_encryption_config = {
    resources = var.cluster_encryption_config_resources

    provider_key_arn = local.enabled && var.cluster_encryption_config_enabled && var.cluster_encryption_config_kms_key_id == "" ? (
      one(aws_kms_key.cluster[*].arn)
    ) : var.cluster_encryption_config_kms_key_id
  }

  cloudwatch_log_group_name = "/aws/eks/${module.label.id}/cluster"
}

module "label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  attributes = var.cluster_attributes

  context = module.this.context
}

data "aws_partition" "current" {
  count = local.enabled ? 1 : 0
}

resource "aws_cloudwatch_log_group" "default" {
  count             = local.enabled && length(var.enabled_cluster_log_types) > 0 ? 1 : 0
  name              = local.cloudwatch_log_group_name
  retention_in_days = var.cluster_log_retention_period
  kms_key_id        = var.cloudwatch_log_group_kms_key_id
  tags              = module.label.tags
  log_group_class   = var.cloudwatch_log_group_class
}

resource "aws_kms_key" "cluster" {
  count                   = local.enabled && var.cluster_encryption_config_enabled && var.cluster_encryption_config_kms_key_id == "" ? 1 : 0
  description             = "EKS Cluster ${module.label.id} Encryption Config KMS Key"
  enable_key_rotation     = var.cluster_encryption_config_kms_key_enable_key_rotation
  deletion_window_in_days = var.cluster_encryption_config_kms_key_deletion_window_in_days
  policy                  = var.cluster_encryption_config_kms_key_policy
  tags                    = module.label.tags
}

resource "aws_kms_alias" "cluster" {
  count         = local.enabled && var.cluster_encryption_config_enabled && var.cluster_encryption_config_kms_key_id == "" ? 1 : 0
  name          = format("alias/%v", module.label.id)
  target_key_id = one(aws_kms_key.cluster[*].key_id)
}

resource "aws_eks_cluster" "default" {
  #bridgecrew:skip=BC_AWS_KUBERNETES_1:Allow permissive security group for public access, difficult to restrict without a VPN
  #bridgecrew:skip=BC_AWS_KUBERNETES_4:Let user decide on control plane logging, not necessary in non-production environments
  count                         = local.enabled ? 1 : 0
  name                          = module.label.id
  tags                          = module.label.tags
  role_arn                      = local.eks_service_role_arn
  version                       = var.kubernetes_version
  enabled_cluster_log_types     = var.enabled_cluster_log_types
  bootstrap_self_managed_addons = var.bootstrap_self_managed_addons_enabled

  access_config {
    authentication_mode                         = var.access_config.authentication_mode
    bootstrap_cluster_creator_admin_permissions = var.access_config.bootstrap_cluster_creator_admin_permissions
  }

  lifecycle {
    # bootstrap_cluster_creator_admin_permissions is documented as only applying
    # to the initial creation of the cluster, and being unreliable afterward,
    # so we want to ignore it except at cluster creation time.
    ignore_changes = [access_config[0].bootstrap_cluster_creator_admin_permissions]
  }

  dynamic "encryption_config" {
    #bridgecrew:skip=BC_AWS_KUBERNETES_3:Let user decide secrets encryption, mainly because changing this value requires completely destroying the cluster
    for_each = var.cluster_encryption_config_enabled ? [local.cluster_encryption_config] : []
    content {
      resources = encryption_config.value.resources
      provider {
        key_arn = encryption_config.value.provider_key_arn
      }
    }
  }

  vpc_config {
    security_group_ids      = var.associated_security_group_ids
    subnet_ids              = var.subnet_ids
    endpoint_private_access = var.endpoint_private_access
    #bridgecrew:skip=BC_AWS_KUBERNETES_2:Let user decide on public access
    endpoint_public_access = var.endpoint_public_access
    public_access_cidrs    = var.public_access_cidrs
  }

  dynamic "kubernetes_network_config" {
    for_each = local.use_ipv6 ? [] : compact([var.service_ipv4_cidr])
    content {
      service_ipv4_cidr = kubernetes_network_config.value
    }
  }

  dynamic "kubernetes_network_config" {
    for_each = local.use_ipv6 ? [true] : []
    content {
      ip_family = "ipv6"
    }
  }

  dynamic "remote_network_config" {
    for_each = var.remote_network_config != null ? [var.remote_network_config] : []

    content {
      dynamic "remote_node_networks" {
        for_each = [remote_network_config.value.remote_node_networks_cidrs]

        content {
          cidrs = remote_network_config.value.remote_node_networks_cidrs
        }
      }

      dynamic "remote_pod_networks" {
        for_each = remote_network_config.value.remote_pod_networks_cidrs != null ? [remote_network_config.value.remote_pod_networks_cidrs] : []

        content {
          cidrs = remote_network_config.value.remote_pod_networks_cidrs
        }
      }
    }
  }

  dynamic "upgrade_policy" {
    for_each = var.upgrade_policy != null ? [var.upgrade_policy] : []
    content {
      support_type = upgrade_policy.value.support_type
    }
  }

  dynamic "zonal_shift_config" {
    for_each = var.zonal_shift_config != null ? [var.zonal_shift_config] : []
    content {
      enabled = zonal_shift_config.value.enabled
    }
  }

  depends_on = [
    aws_iam_role.default,
    aws_iam_role_policy_attachment.cluster_elb_service_role,
    aws_iam_role_policy_attachment.amazon_eks_cluster_policy,
    aws_iam_role_policy_attachment.amazon_eks_service_policy,
    aws_kms_alias.cluster,
    aws_cloudwatch_log_group.default,
    var.associated_security_group_ids,
    var.cluster_depends_on,
    var.subnet_ids,
  ]
}

# Enabling IAM Roles for Service Accounts in Kubernetes cluster
#
# From official docs:
# The IAM roles for service accounts feature is available on new Amazon EKS Kubernetes version 1.14 clusters,
# and clusters that were updated to versions 1.14 or 1.13 on or after September 3rd, 2019.
#
# https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html
# https://medium.com/@marcincuber/amazon-eks-with-oidc-provider-iam-roles-for-kubernetes-services-accounts-59015d15cb0c
#

data "tls_certificate" "cluster" {
  count = local.enabled && var.oidc_provider_enabled ? 1 : 0
  url   = one(aws_eks_cluster.default[*].identity[0].oidc[0].issuer)
}

resource "aws_iam_openid_connect_provider" "default" {
  count = local.enabled && var.oidc_provider_enabled ? 1 : 0
  url   = one(aws_eks_cluster.default[*].identity[0].oidc[0].issuer)
  tags  = module.label.tags

  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [one(data.tls_certificate.cluster[*].certificates[0].sha1_fingerprint)]
}

resource "aws_eks_addon" "cluster" {
  for_each = local.enabled ? {
    for addon in var.addons :
    addon.addon_name => addon
  } : {}

  cluster_name                = one(aws_eks_cluster.default[*].name)
  addon_name                  = each.key
  addon_version               = lookup(each.value, "addon_version", null)
  configuration_values        = lookup(each.value, "configuration_values", null)
  resolve_conflicts_on_create = lookup(each.value, "resolve_conflicts_on_create", try(replace(each.value.resolve_conflicts, "PRESERVE", "NONE"), null))
  resolve_conflicts_on_update = lookup(each.value, "resolve_conflicts_on_update", lookup(each.value, "resolve_conflicts", null))
  service_account_role_arn    = lookup(each.value, "service_account_role_arn", null)

  dynamic "pod_identity_association" {
    for_each = merge({}, lookup(each.value, "pod_identity_association", {}))

    content {
      service_account = pod_identity_association.key
      role_arn        = pod_identity_association.value
    }
  }

  tags = merge(module.label.tags, each.value.additional_tags)

  depends_on = [
    var.addons_depends_on,
    aws_eks_cluster.default,
    # OIDC provider is prerequisite for some addons. See, for example,
    # https://docs.aws.amazon.com/eks/latest/userguide/managing-vpc-cni.html
    aws_iam_openid_connect_provider.default,
  ]

  timeouts {
    create = each.value.create_timeout
    update = each.value.update_timeout
    delete = each.value.delete_timeout
  }
}
