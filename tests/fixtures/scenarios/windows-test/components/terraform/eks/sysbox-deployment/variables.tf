variable "region" {
  description = "AWS Region."
  type        = string
}

variable "eks_cluster_id" {
  type        = string
  description = "EKS cluster ID for Kubernetes provider configuration"
}

variable "kubernetes_namespace" {
  type        = string
  description = "Kubernetes namespace for the deployment. Defaults to module.this.name if not set."
  default     = null
}

variable "create_namespace" {
  type        = bool
  description = "Whether to create the namespace"
  default     = true
}

variable "deployment_name" {
  type        = string
  description = "Name of the deployment. Defaults to module.this.name if not set."
  default     = null
}

variable "replicas" {
  type        = number
  description = "Number of replicas for the deployment"
  default     = 1
}

variable "runtime_class_name" {
  type        = string
  description = "RuntimeClass name to use for the pods (from eks/sysbox-runtime output)"
}

variable "container_image" {
  type        = string
  description = "Container image for the pod"
  default     = "nestybox/ubuntu-jammy-systemd-docker:latest"
}

variable "container_command" {
  type        = list(string)
  description = "Command to run in the container"
  default     = ["/sbin/init"]
}

variable "resources_requests_cpu" {
  type        = string
  description = "CPU request for the container"
  default     = "100m"
}

variable "resources_requests_memory" {
  type        = string
  description = "Memory request for the container"
  default     = "256Mi"
}

variable "resources_limits_cpu" {
  type        = string
  description = "CPU limit for the container"
  default     = "500m"
}

variable "resources_limits_memory" {
  type        = string
  description = "Memory limit for the container"
  default     = "512Mi"
}
