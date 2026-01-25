variable "region" {
  description = "AWS Region."
  type        = string
}

variable "eks_cluster_id" {
  type        = string
  description = "EKS cluster ID for Kubernetes provider configuration"
}

variable "eks_cluster_endpoint" {
  type        = string
  description = "EKS cluster endpoint URL"
}

variable "eks_cluster_certificate_authority_data" {
  type        = string
  description = "Base64-encoded EKS cluster CA certificate"
}

variable "runtime_class_name" {
  type        = string
  description = "Name of the RuntimeClass resource. Pods use this via runtimeClassName."
  default     = "sysbox-runc"
  nullable    = false
}

variable "runtime_handler" {
  type        = string
  description = "The handler that should be used by the container runtime. Must match CRI-O config."
  default     = "sysbox-runc"
  nullable    = false
}

variable "sysbox_deploy_image" {
  type        = string
  description = "Docker image for sysbox-deploy-k8s DaemonSet. This installs CRI-O and Sysbox on labeled nodes."
  default     = "registry.nestybox.com/nestybox/sysbox-deploy-k8s:v0.6.7-0"
  nullable    = false
}

variable "node_selector" {
  type        = map(string)
  description = "Node selector to schedule pods using this RuntimeClass only on Sysbox-enabled nodes. Uses sysbox-install=yes which is set at node creation time."
  default = {
    "sysbox-install" = "yes"
  }
  nullable = false
}

variable "tolerations" {
  type = list(object({
    key      = string
    operator = string
    value    = optional(string)
    effect   = string
  }))
  description = "Tolerations that will be added to pods using this RuntimeClass"
  default = [
    {
      key      = "sandbox-runtime"
      operator = "Equal"
      value    = "sysbox"
      effect   = "NoSchedule"
    }
  ]
  nullable = false
}

variable "overhead_pod_fixed" {
  type        = map(string)
  description = "Fixed resource overhead for Sysbox pods. Memory and CPU overhead per pod."
  default = {
    "memory" = "350Mi"
    "cpu"    = "250m"
  }
  nullable = false
}
