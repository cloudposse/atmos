# tflint-ignore: terraform_unused_declarations
variable "region" {

  type        = string
  description = "OBSOLETE (not needed): AWS Region"
  default     = null
}

variable "subnet_ids" {
  type        = list(string)
  description = "A list of subnet IDs to launch the cluster in"
}

variable "associated_security_group_ids" {
  type        = list(string)
  default     = []
  description = <<-EOT
    A list of IDs of Security Groups to associate the cluster with.
    These security groups will not be modified.
    EOT
}

variable "cluster_depends_on" {
  type        = any
  description = <<-EOT
    If provided, the EKS will depend on this object, and therefore not be created until this object is finalized.
    This is useful if you want to ensure that the cluster is not created before some other condition is met, e.g. VPNs into the subnet are created.
    EOT
  default     = null
}

variable "create_eks_service_role" {
  type        = bool
  description = "Set `false` to use existing `eks_cluster_service_role_arn` instead of creating one"
  default     = true
}

variable "eks_cluster_service_role_arn" {
  type        = string
  description = <<-EOT
    The ARN of an IAM role for the EKS cluster to use that provides permissions
    for the Kubernetes control plane to perform needed AWS API operations.
    Required if `create_eks_service_role` is `false`, ignored otherwise.
    EOT
  default     = null
}


variable "kubernetes_version" {
  type        = string
  description = "Desired Kubernetes master version. If you do not specify a value, the latest available version is used"
  default     = "1.21"
}

variable "oidc_provider_enabled" {
  type        = bool
  description = <<-EOT
    Create an IAM OIDC identity provider for the cluster, then you can create IAM roles to associate with a
    service account in the cluster, instead of using kiam or kube2iam. For more information,
    see [EKS User Guide](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).
    EOT
  default     = false
}

variable "endpoint_private_access" {
  type        = bool
  description = "Indicates whether or not the Amazon EKS private API server endpoint is enabled. Default to AWS EKS resource and it is false"
  default     = false
}

variable "endpoint_public_access" {
  type        = bool
  description = "Indicates whether or not the Amazon EKS public API server endpoint is enabled. Default to AWS EKS resource and it is true"
  default     = true
}

variable "public_access_cidrs" {
  type        = list(string)
  description = "Indicates which CIDR blocks can access the Amazon EKS public API server endpoint when enabled. EKS defaults this to a list with 0.0.0.0/0."
  default     = ["0.0.0.0/0"]
}

variable "service_ipv4_cidr" {
  type        = string
  description = <<-EOT
    The CIDR block to assign Kubernetes service IP addresses from.
    You can only specify a custom CIDR block when you create a cluster, changing this value will force a new cluster to be created.
    EOT
  default     = null
}

variable "kubernetes_network_ipv6_enabled" {
  type        = bool
  description = "Set true to use IPv6 addresses for Kubernetes pods and services"
  default     = false
}

variable "enabled_cluster_log_types" {
  type        = list(string)
  description = "A list of the desired control plane logging to enable. For more information, see https://docs.aws.amazon.com/en_us/eks/latest/userguide/control-plane-logs.html. Possible values [`api`, `audit`, `authenticator`, `controllerManager`, `scheduler`]"
  default     = []
}

variable "cluster_log_retention_period" {
  type        = number
  description = "Number of days to retain cluster logs. Requires `enabled_cluster_log_types` to be set. See https://docs.aws.amazon.com/en_us/eks/latest/userguide/control-plane-logs.html."
  default     = 0
}

variable "cluster_encryption_config_enabled" {
  type        = bool
  description = "Set to `true` to enable Cluster Encryption Configuration"
  default     = true
}

variable "cluster_encryption_config_kms_key_id" {
  type        = string
  description = "KMS Key ID to use for cluster encryption config"
  default     = ""
}

variable "cluster_encryption_config_kms_key_enable_key_rotation" {
  type        = bool
  description = "Cluster Encryption Config KMS Key Resource argument - enable kms key rotation"
  default     = true
}

variable "cluster_encryption_config_kms_key_deletion_window_in_days" {
  type        = number
  description = "Cluster Encryption Config KMS Key Resource argument - key deletion windows in days post destruction"
  default     = 10
}

variable "cluster_encryption_config_kms_key_policy" {
  type        = string
  description = "Cluster Encryption Config KMS Key Resource argument - key policy"
  default     = null
}

variable "cluster_encryption_config_resources" {
  type        = list(any)
  description = "Cluster Encryption Config Resources to encrypt, e.g. ['secrets']"
  default     = ["secrets"]
}

variable "permissions_boundary" {
  type        = string
  description = "If provided, all IAM roles will be created with this permissions boundary attached"
  default     = null
}

variable "cloudwatch_log_group_kms_key_id" {
  type        = string
  description = "If provided, the KMS Key ID to use to encrypt AWS CloudWatch logs"
  default     = null
}

variable "cloudwatch_log_group_class" {
  type        = string
  description = "Specified the log class of the log group. Possible values are: `STANDARD` or `INFREQUENT_ACCESS`"
  default     = null
}

variable "addons" {
  type = list(object({
    addon_name           = string
    addon_version        = optional(string, null)
    configuration_values = optional(string, null)
    # resolve_conflicts is deprecated, but we keep it for backwards compatibility
    # and because if not declared, Terraform will silently ignore it.
    resolve_conflicts           = optional(string, null)
    resolve_conflicts_on_create = optional(string, null)
    resolve_conflicts_on_update = optional(string, null)
    service_account_role_arn    = optional(string, null)
    pod_identity_association    = optional(map(string), {})
    create_timeout              = optional(string, null)
    update_timeout              = optional(string, null)
    delete_timeout              = optional(string, null)
    additional_tags             = optional(map(string), {})
  }))
  description = <<-EOT
    Manages [`aws_eks_addon`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_addon) resources.
    Note: `resolve_conflicts` is deprecated. If `resolve_conflicts` is set and
    `resolve_conflicts_on_create` or `resolve_conflicts_on_update` is not set,
    `resolve_conflicts` will be used instead. If `resolve_conflicts_on_create` is
    not set and `resolve_conflicts` is `PRESERVE`, `resolve_conflicts_on_create`
    will be set to `NONE`.
    If `additional_tags` are specified, they are added to the standard resource tags.
    EOT
  default     = []
}

variable "addons_depends_on" {
  type        = any
  description = <<-EOT
    If provided, all addons will depend on this object, and therefore not be installed until this object is finalized.
    This is useful if you want to ensure that addons are not applied before some other condition is met, e.g. node groups are created.
    See [issue #170](https://github.com/cloudposse/terraform-aws-eks-cluster/issues/170) for more details.
    EOT
  default     = null
}

variable "bootstrap_self_managed_addons_enabled" {
  description = "Manages bootstrap of default networking addons after cluster has been created"
  type        = bool
  default     = null
}

variable "upgrade_policy" {
  type = object({
    support_type = optional(string, null)
  })
  description = "Configuration block for the support policy to use for the cluster"
  default     = null
}

variable "zonal_shift_config" {
  type = object({
    enabled = optional(bool, null)
  })
  description = "Configuration block with zonal shift configuration for the cluster"
  default     = null
}

variable "cluster_attributes" {
  type        = list(string)
  description = "Override label module default cluster attributes"
  default     = ["cluster"]
}

variable "access_config" {
  type = object({
    authentication_mode                         = optional(string, "API")
    bootstrap_cluster_creator_admin_permissions = optional(bool, false)
  })
  description = "Access configuration for the EKS cluster."
  default     = {}
  nullable    = false

  validation {
    condition     = !contains(["CONFIG_MAP"], var.access_config.authentication_mode)
    error_message = "The CONFIG_MAP authentication_mode is not supported."
  }
}

variable "access_entry_map" {
  type = map(object({
    # key is principal_arn
    user_name = optional(string)
    # Cannot assign "system:*" groups to IAM users, use ClusterAdmin and Admin instead
    kubernetes_groups = optional(list(string), [])
    type              = optional(string, "STANDARD")
    access_policy_associations = optional(map(object({
      # key is policy_arn or policy_name
      access_scope = optional(object({
        type       = optional(string, "cluster")
        namespaces = optional(list(string))
      }), {}) # access_scope
    })), {})  # access_policy_associations
  }))         # access_entry_map
  description = <<-EOT
    Map of IAM Principal ARNs to access configuration.
    Preferred over other inputs as this configuration remains stable
    when elements are added or removed, but it requires that the Principal ARNs
    and Policy ARNs are known at plan time.
    Can be used along with other `access_*` inputs, but do not duplicate entries.
    Map `access_policy_associations` keys are policy ARNs, policy
    full name (AmazonEKSViewPolicy), or short name (View).
    It is recommended to use the default `user_name` because the default includes
    IAM role or user name and the session name for assumed roles.
    As a special case in support of backwards compatibility, membership in the
    `system:masters` group is is translated to an association with the ClusterAdmin policy.
    In all other cases, including any `system:*` group in `kubernetes_groups` is prohibited.
    EOT
  default     = {}
  nullable    = false
}

variable "access_entries" {
  type = list(object({
    principal_arn     = string
    user_name         = optional(string, null)
    kubernetes_groups = optional(list(string), null)
  }))
  description = <<-EOT
    List of IAM principles to allow to access the EKS cluster.
    It is recommended to use the default `user_name` because the default includes
    the IAM role or user name and the session name for assumed roles.
    Use when Principal ARN is not known at plan time.
    EOT
  default     = []
  nullable    = false
}

variable "access_policy_associations" {
  type = list(object({
    principal_arn = string
    policy_arn    = string
    access_scope = optional(object({
      type       = optional(string, "cluster")
      namespaces = optional(list(string))
    }), {})
  }))
  description = <<-EOT
    List of AWS managed EKS access policies to associate with IAM principles.
    Use when Principal ARN or Policy ARN is not known at plan time.
    `policy_arn` can be the full ARN, the full name (AmazonEKSViewPolicy) or short name (View).
    EOT
  default     = []
  nullable    = false
}

variable "access_entries_for_nodes" {
  # We use a map instead of an object because if a user supplies
  # an object with an unexpected key, Terraform simply ignores it,
  # leaving us with no way to detect the error.
  type        = map(list(string))
  description = <<-EOT
    Map of list of IAM roles for the EKS non-managed worker nodes.
    The map key is the node type, either `EC2_LINUX` or `EC2_WINDOWS`,
    and the list contains the IAM roles of the nodes of that type.
    There is no need for or utility in creating Fargate access entries, as those
    are always created automatically by AWS, just as with managed nodes.
    Use when Principal ARN is not known at plan time.
    EOT
  default     = {}
  nullable    = false
  validation {
    condition = length([for k in keys(var.access_entries_for_nodes) : k if !contains(["EC2_LINUX", "EC2_WINDOWS"], k)]) == 0
    error_message = format(<<-EOS
      The access_entries_for_nodes object can only contain the EC2_LINUX and EC2_WINDOWS attributes:
      Keys "%s" not allowed.
      EOS
    , join("\", \"", [for k in keys(var.access_entries_for_nodes) : k if !contains(["EC2_LINUX", "EC2_WINDOWS"], k)]))
  }
  validation {
    condition     = !(contains(keys(var.access_entries_for_nodes), "FARGATE_LINUX"))
    error_message = <<-EOM
      Access entries of type "FARGATE_LINUX" are not supported because they are
      automatically created by AWS EKS and should not be managed by Terraform.
      EOM
  }
}

## Limited support for modifying the EKS-managed Security Group
## In the future, even this limited support may be removed

variable "managed_security_group_rules_enabled" {
  type        = bool
  description = "Flag to enable/disable the ingress and egress rules for the EKS managed Security Group"
  default     = true
}

variable "allowed_security_group_ids" {
  type        = list(string)
  default     = []
  description = <<-EOT
    A list of IDs of Security Groups to allow access to the cluster.
    EOT
}

variable "allowed_cidr_blocks" {
  type        = list(string)
  default     = []
  description = <<-EOT
    A list of IPv4 CIDRs to allow access to the cluster.
    The length of this list must be known at "plan" time.
    EOT
}

variable "custom_ingress_rules" {
  type = list(object({
    description              = string
    from_port                = number
    to_port                  = number
    protocol                 = string
    source_security_group_id = string
  }))
  default     = []
  description = <<-EOT
    A List of Objects, which are custom security group rules that
    EOT
}

variable "remote_network_config" {
  description = "Configuration block for the cluster remote network configuration"
  type = object({
    remote_node_networks_cidrs = list(string)
    remote_pod_networks_cidrs  = optional(list(string))
  })
  default = null
}
