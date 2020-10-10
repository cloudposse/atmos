variable "region" {
  type        = string
  description = "AWS Region"
}

variable "region_availability_zones" {
  type        = list(string)
  description = "AWS Availability Zones in which to deploy multi-AZ resources"
}

variable "oidc_provider_enabled" {
  type        = bool
  description = "Create an IAM OIDC identity provider for the cluster, then you can create IAM roles to associate with a service account in the cluster, instead of using kiam or kube2iam. For more information, see https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html"
}

variable "cluster_endpoint_private_access" {
  type        = bool
  default     = true
  description = "Indicates whether or not the Amazon EKS private API server endpoint is enabled. Default to AWS EKS resource and it is `false`"
}

variable "cluster_endpoint_public_access" {
  type        = bool
  default     = true
  description = "Indicates whether or not the Amazon EKS public API server endpoint is enabled. Default to AWS EKS resource and it is `true`"
}

variable "cluster_kubernetes_version" {
  type        = string
  default     = null
  description = "Desired Kubernetes master version. If you do not specify a value, the latest available version is used"
}

variable "public_access_cidrs" {
  type        = list(string)
  default     = ["0.0.0.0/0"]
  description = "Indicates which CIDR blocks can access the Amazon EKS public API server endpoint when enabled. EKS defaults this to a list with 0.0.0.0/0."
}

variable "enabled_cluster_log_types" {
  type        = list(string)
  default     = []
  description = "A list of the desired control plane logging to enable. For more information, see https://docs.aws.amazon.com/en_us/eks/latest/userguide/control-plane-logs.html. Possible values [`api`, `audit`, `authenticator`, `controllerManager`, `scheduler`]"
}

variable "cluster_log_retention_period" {
  type        = number
  default     = 0
  description = "Number of days to retain cluster logs. Requires `enabled_cluster_log_types` to be set. See https://docs.aws.amazon.com/en_us/eks/latest/userguide/control-plane-logs.html."
}

variable "apply_config_map_aws_auth" {
  type        = bool
  default     = true
  description = "Whether to execute `kubectl apply` to apply the ConfigMap to allow worker nodes to join the EKS cluster"
}

variable "map_additional_aws_accounts" {
  description = "Additional AWS account numbers to add to `config-map-aws-auth` ConfigMap"
  type        = list(string)
  default     = []
}

variable "map_additional_iam_users" {
  description = "Additional IAM users to add to `config-map-aws-auth` ConfigMap"

  type = list(object({
    userarn  = string
    username = string
    groups   = list(string)
  }))

  default = []
}

variable "allowed_security_groups" {
  type        = list(string)
  default     = []
  description = "List of Security Group IDs to be allowed to connect to the EKS cluster"
}

variable "allowed_cidr_blocks" {
  type        = list(string)
  default     = []
  description = "List of CIDR blocks to be allowed to connect to the EKS cluster"
}

variable "subnet_type_tag_key" {
  type        = string
  description = "The tag used to find the private subnets to find by availability zone"
}

variable "enable_vpn_access" {
  type        = bool
  default     = false
  description = "Enable VPN access via the HAL VPN; see vpn project"
}

variable "node_groups" {
  # will create 1 node group for each item in map
  type = map(object({
    # will create 1 auto scaling group in each specified availability zone
    availability_zones = list(string)
    # Additional attributes (e.g. `1`) for the node group
    attributes = list(string)
    # True to create new node_groups before deleting old ones, avoiding a temporary outage
    create_before_destroy = bool
    # Desired number of worker nodes when initially provisioned
    desired_group_size = number
    # Disk size in GiB for worker nodes. Terraform will only perform drift detection if a configuration value is provided.
    disk_size = number
    # Whether to enable Node Group to scale its AutoScaling Group
    enable_cluster_autoscaler = bool
    # Set of instance types associated with the EKS Node Group. Terraform will only perform drift detection if a configuration value is provided.
    instance_types = list(string)
    # Type of Amazon Machine Image (AMI) associated with the EKS Node Group
    ami_type = string
    # EKS AMI version to use, e.g. "1.16.13-20200821" (no "v").
    ami_release_version = string
    # Key-value mapping of Kubernetes labels. Only labels that are applied with the EKS API are managed by this argument. Other Kubernetes labels applied to the EKS Node Group will not be managed
    kubernetes_labels = map(string)
    # Key-value mapping of Kubernetes taints.
    kubernetes_taints = map(string)
    # Desired Kubernetes master version. If you do not specify a value, the latest available version is used
    kubernetes_version = string
    # The maximum size of the AutoScaling Group
    max_group_size = number
    # The minimum size of the AutoScaling Group
    min_group_size = number
    # List of auto-launched resource types to tag
    resources_to_tag = list(string)
    tags             = map(string)
  }))
  description = "List of objects defining a node group for the cluster"
  default     = null
}

variable "node_group_defaults" {
  # Any value in the node group that is null will be replaced
  # by the value in this object, which can also be null
  type = object({
    availability_zones        = list(string) # set to null to use var.region_availability_zones
    attributes                = list(string)
    create_before_destroy     = bool
    desired_group_size        = number
    disk_size                 = number
    enable_cluster_autoscaler = bool
    instance_types            = list(string)
    ami_type                  = string
    ami_release_version       = string
    kubernetes_version        = string # set to null to use cluster_kubernetes_version
    kubernetes_labels         = map(string)
    kubernetes_taints         = map(string)
    max_group_size            = number
    min_group_size            = number
    resources_to_tag          = list(string)
    tags                      = map(string)
  })
  description = "Defaults for node groups in the cluster"
}
