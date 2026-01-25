##################
#
# Kubernetes and Helm provider configuration.
#
# This component receives cluster details via variables populated using
# !terraform.state in stack config, rather than remote-state module.
#

variable "eks_cluster_endpoint" {
  type        = string
  description = "EKS cluster endpoint URL"
}

variable "eks_cluster_certificate_authority_data" {
  type        = string
  description = "Base64-encoded EKS cluster CA certificate"
}

variable "kubeconfig_file_enabled" {
  type        = bool
  default     = false
  description = "If `true`, configure the Kubernetes provider with `kubeconfig_file`"
}

variable "kubeconfig_file" {
  type        = string
  default     = ""
  description = "The Kubernetes provider `config_path` setting to use when `kubeconfig_file_enabled` is `true`"
}

variable "kubeconfig_context" {
  type        = string
  default     = ""
  description = "Context to choose from the Kubernetes config file"
}

variable "kube_exec_auth_enabled" {
  type        = bool
  default     = true
  description = "If `true`, use the Kubernetes provider `exec` feature to execute `aws eks get-token`"
}

variable "kube_exec_auth_role_arn" {
  type        = string
  default     = ""
  description = "The role ARN for `aws eks get-token` to use"
}

variable "kube_exec_auth_role_arn_enabled" {
  type        = bool
  default     = false
  description = "If `true`, pass `kube_exec_auth_role_arn` as the role ARN to `aws eks get-token`"
}

variable "kubeconfig_exec_auth_api_version" {
  type        = string
  default     = "client.authentication.k8s.io/v1beta1"
  description = "The Kubernetes API version of the credentials returned by the `exec` auth plugin"
}

locals {
  kubeconfig_file_enabled = var.kubeconfig_file_enabled
  kubeconfig_file         = local.kubeconfig_file_enabled ? var.kubeconfig_file : ""
  kubeconfig_context      = var.kubeconfig_context

  kube_exec_auth_enabled = local.kubeconfig_file_enabled ? false : var.kube_exec_auth_enabled

  exec_role = var.kube_exec_auth_enabled && var.kube_exec_auth_role_arn_enabled && var.kube_exec_auth_role_arn != "" ? [
    "--role-arn", var.kube_exec_auth_role_arn
  ] : []

  cluster_ca_certificate = local.kubeconfig_file_enabled ? null : try(base64decode(var.eks_cluster_certificate_authority_data), null)
  eks_cluster_endpoint   = local.kubeconfig_file_enabled ? null : var.eks_cluster_endpoint
}

provider "helm" {
  kubernetes {
    host                   = local.eks_cluster_endpoint
    cluster_ca_certificate = local.cluster_ca_certificate
    config_path            = local.kubeconfig_file
    config_context         = local.kubeconfig_context

    dynamic "exec" {
      for_each = local.kube_exec_auth_enabled && local.cluster_ca_certificate != null ? ["exec"] : []
      content {
        api_version = var.kubeconfig_exec_auth_api_version
        command     = "aws"
        args = concat([
          "eks", "get-token", "--cluster-name", var.eks_cluster_id
        ], local.exec_role)
      }
    }
  }
}

provider "kubernetes" {
  host                   = local.eks_cluster_endpoint
  cluster_ca_certificate = local.cluster_ca_certificate
  config_path            = local.kubeconfig_file
  config_context         = local.kubeconfig_context

  dynamic "exec" {
    for_each = local.kube_exec_auth_enabled && local.cluster_ca_certificate != null ? ["exec"] : []
    content {
      api_version = var.kubeconfig_exec_auth_api_version
      command     = "aws"
      args = concat([
        "eks", "get-token", "--cluster-name", var.eks_cluster_id
      ], local.exec_role)
    }
  }
}
