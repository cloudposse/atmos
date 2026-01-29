# EKS Component - Mock for testing

variable "cluster_name" {
  type        = string
  default     = "eks-cluster"
  description = "Name of the EKS cluster"
}

variable "kubernetes_version" {
  type        = string
  default     = "1.28"
  description = "Kubernetes version"
}

variable "node_count" {
  type        = number
  default     = 3
  description = "Number of worker nodes"
}

output "cluster_name" {
  value       = var.cluster_name
  description = "The EKS cluster name"
}

output "kubernetes_version" {
  value       = var.kubernetes_version
  description = "The Kubernetes version"
}
