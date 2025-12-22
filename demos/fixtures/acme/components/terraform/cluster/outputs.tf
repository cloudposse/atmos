output "eks_cluster_id" {
  description = "The name of the cluster"
  value       = one(aws_eks_cluster.default[*].id)
}

output "eks_cluster_arn" {
  description = "The Amazon Resource Name (ARN) of the cluster"
  value       = one(aws_eks_cluster.default[*].arn)
}

output "eks_cluster_endpoint" {
  description = "The endpoint for the Kubernetes API server"
  value       = one(aws_eks_cluster.default[*].endpoint)
}

output "eks_cluster_version" {
  description = "The Kubernetes server version of the cluster"
  value       = one(aws_eks_cluster.default[*].version)
}

output "eks_cluster_identity_oidc_issuer" {
  description = "The OIDC Identity issuer for the cluster"
  value       = one(aws_eks_cluster.default[*].identity[0].oidc[0].issuer)
}

output "eks_cluster_identity_oidc_issuer_arn" {
  description = "The OIDC Identity issuer ARN for the cluster that can be used to associate IAM roles with a service account"
  value       = one(aws_iam_openid_connect_provider.default[*].arn)
}

output "eks_cluster_certificate_authority_data" {
  description = "The Kubernetes cluster certificate authority data"
  value       = local.certificate_authority_data
}

output "eks_cluster_managed_security_group_id" {
  description = <<-EOT
    Security Group ID that was created by EKS for the cluster.
    EKS creates a Security Group and applies it to the ENI that are attached to EKS Control Plane master nodes and to any managed workloads.
    EOT
  value       = one(aws_eks_cluster.default[*].vpc_config[0].cluster_security_group_id)
}

output "eks_cluster_role_arn" {
  description = "ARN of the EKS cluster IAM role"
  value       = local.eks_service_role_arn
}

output "eks_cluster_ipv4_service_cidr" {
  description = <<-EOT
    The IPv4 CIDR block that Kubernetes pod and service IP addresses are assigned from
    if `kubernetes_network_ipv6_enabled` is set to false. If set to true this output will be null.
    EOT
  value       = one(aws_eks_cluster.default[*].kubernetes_network_config[0].service_ipv4_cidr)
}

output "eks_cluster_ipv6_service_cidr" {
  description = <<-EOT
    The IPv6 CIDR block that Kubernetes pod and service IP addresses are assigned from
    if `kubernetes_network_ipv6_enabled` is set to true. If set to false this output will be null.
    EOT
  value       = one(aws_eks_cluster.default[*].kubernetes_network_config[0].service_ipv6_cidr)
}

output "eks_addons_versions" {
  description = "Map of enabled EKS Addons names and versions"
  value = local.enabled ? {
    for addon in aws_eks_addon.cluster :
    addon.addon_name => addon.addon_version
  } : {}
}

output "cluster_encryption_config_enabled" {
  description = "If true, Cluster Encryption Configuration is enabled"
  value       = var.cluster_encryption_config_enabled
}

output "cluster_encryption_config_resources" {
  description = "Cluster Encryption Config Resources"
  value       = var.cluster_encryption_config_resources
}

output "cluster_encryption_config_provider_key_arn" {
  description = "Cluster Encryption Config KMS Key ARN"
  value       = local.cluster_encryption_config.provider_key_arn
}

output "cluster_encryption_config_provider_key_alias" {
  description = "Cluster Encryption Config KMS Key Alias ARN"
  value       = one(aws_kms_alias.cluster[*].arn)
}

output "cloudwatch_log_group_name" {
  description = "The name of the log group created in cloudwatch where cluster logs are forwarded to if enabled"
  value       = local.cloudwatch_log_group_name
}

output "cloudwatch_log_group_kms_key_id" {
  description = "KMS Key ID to encrypt AWS CloudWatch logs"
  value       = var.cloudwatch_log_group_kms_key_id
}
