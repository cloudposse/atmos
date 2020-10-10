# This file is included by default in terraform plans

namespace = "eg"

name = "eks"

allowed_security_groups = []

allowed_cidr_blocks = []

cluster_log_retention_period = 90

cluster_endpoint_private_access = true

cluster_endpoint_public_access = true

cluster_kubernetes_version = "1.17"

enabled_cluster_log_types = ["api", "audit", "authenticator", "controllerManager", "scheduler"]

oidc_provider_enabled = true


# EKS IAM Authentication settings
# By default, you can authenticate to EKS cluster only by assuming the role that created the cluster.
# In order to apply the Auth Config Map to allow other roles to login to the cluster,
# `kubectl` will need to assume the same role that created the cluster - that's why we are setting
# aws_cli_assume_role_arn in each stage to the `tzl-gbl-${stage}-terraform` role.
# After the Auth Config Map is applied, the other IAM roles in
# `primary_additional_iam_roles` and `map_additional_iam_roles` will be able to authenticate.
apply_config_map_aws_auth = true

public_access_cidrs = ["0.0.0.0/0"]

node_group_defaults = {
  availability_zones = null # use default region_availability_zones

  desired_group_size = 3 # number of instances to start with, must be >= number of AZs
  max_group_size     = 12
  min_group_size     = 3

  kubernetes_version  = null # use cluster_kubernetes_version
  ami_release_version = null # use latest for given Kubernetes version

  attributes                = []
  create_before_destroy     = true
  disk_size                 = 100 # root EBS volume size in GB
  enable_cluster_autoscaler = true
  instance_types            = ["t3.medium"]
  ami_type                  = "AL2_x86_64" # use "AL2_x86_64_GPU" for GPU instances
  kubernetes_labels         = {}
  kubernetes_taints         = {}
  resources_to_tag          = ["instance", "volume"]
  tags                      = null
}
