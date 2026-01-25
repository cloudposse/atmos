# EKS OIDC / IRSA support for IAM roles
# Allows Kubernetes service accounts to assume this IAM role

variable "eks_oidc_enabled" {
  type        = bool
  description = "Enable EKS OIDC provider for IRSA (IAM Roles for Service Accounts)"
  default     = false
}

variable "eks_cluster_oidc_issuer_url" {
  type        = string
  description = "The OIDC issuer URL from the EKS cluster (e.g., https://oidc.eks.us-west-2.amazonaws.com/id/XXXXX)"
  default     = ""
}

variable "service_account_name" {
  type        = string
  description = "The name of the Kubernetes service account allowed to assume this role. Use '*' to allow any service account in the namespace. Defaults to module.this.name if not specified."
  default     = null
}

variable "service_account_namespace" {
  type        = string
  description = "The Kubernetes namespace of the service account allowed to assume this role. Defaults to module.this.name if not specified."
  default     = null
}

locals {
  eks_oidc_enabled = local.enabled && var.eks_oidc_enabled

  # Default service account namespace and name to the component name if not specified
  service_account_namespace = coalesce(var.service_account_namespace, module.this.name)
  service_account_name      = coalesce(var.service_account_name, module.this.name)

  # Extract OIDC issuer URL (remove https:// prefix if present)
  eks_oidc_issuer_url = local.eks_oidc_enabled ? replace(var.eks_cluster_oidc_issuer_url, "https://", "") : ""

  # Construct the OIDC provider ARN
  eks_oidc_provider_arn = local.eks_oidc_enabled ? "arn:aws:iam::${data.aws_caller_identity.current[0].account_id}:oidc-provider/${local.eks_oidc_issuer_url}" : ""

  # Construct the subject claim for the service account
  # Format: system:serviceaccount:<namespace>:<service-account-name>
  eks_service_account_subject = local.eks_oidc_enabled ? "system:serviceaccount:${local.service_account_namespace}:${local.service_account_name}" : ""
}

data "aws_caller_identity" "current" {
  count = local.eks_oidc_enabled ? 1 : 0
}

data "aws_iam_policy_document" "eks_oidc_provider_assume" {
  count = local.eks_oidc_enabled ? 1 : 0

  statement {
    sid = "EksOidcProviderAssume"
    actions = [
      "sts:AssumeRoleWithWebIdentity",
      "sts:TagSession",
    ]

    principals {
      type        = "Federated"
      identifiers = [local.eks_oidc_provider_arn]
    }

    # Verify the audience is AWS STS
    condition {
      test     = "StringEquals"
      variable = "${local.eks_oidc_issuer_url}:aud"
      values   = ["sts.amazonaws.com"]
    }

    # Restrict to specific namespace and service account
    # Use StringLike to support wildcards in service account name
    condition {
      test     = local.service_account_name == "*" ? "StringLike" : "StringEquals"
      variable = "${local.eks_oidc_issuer_url}:sub"
      values   = [local.eks_service_account_subject]
    }
  }
}

output "eks_assume_role_policy" {
  value       = local.eks_oidc_enabled ? one(data.aws_iam_policy_document.eks_oidc_provider_assume[*].json) : null
  description = "JSON encoded string representing the EKS OIDC \"Assume Role\" policy"
}

output "eks_oidc_provider_arn" {
  value       = local.eks_oidc_provider_arn
  description = "ARN of the EKS OIDC provider"
}

output "eks_service_account_subject" {
  value       = local.eks_service_account_subject
  description = "The service account subject claim used in the trust policy"
}
