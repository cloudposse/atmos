# Frontend Component
# Stub component for demo purposes

variable "enabled" {
  type        = bool
  default     = true
  description = "Enable frontend"
}

variable "name" {
  type        = string
  description = "Frontend application name"
}

variable "image_repository" {
  type        = string
  default     = ""
  description = "Container image repository"
}

variable "image_tag" {
  type        = string
  default     = "latest"
  description = "Container image tag"
}

variable "replicas" {
  type        = number
  default     = 3
  description = "Number of replicas"
}

variable "cpu_request" {
  type        = string
  default     = "128m"
  description = "CPU request"
}

variable "memory_request" {
  type        = string
  default     = "256Mi"
  description = "Memory request"
}

variable "cpu_limit" {
  type        = string
  default     = "500m"
  description = "CPU limit"
}

variable "memory_limit" {
  type        = string
  default     = "1Gi"
  description = "Memory limit"
}

variable "health_check_path" {
  type        = string
  default     = "/"
  description = "Health check path"
}

variable "static_assets_bucket" {
  type        = bool
  default     = false
  description = "Create static assets bucket"
}

variable "cdn_enabled" {
  type        = bool
  default     = false
  description = "Enable CDN"
}

variable "autoscaling" {
  type = object({
    enabled                = bool
    min_replicas           = number
    max_replicas           = number
    target_cpu_utilization = number
  })
  default = {
    enabled                = false
    min_replicas           = 1
    max_replicas           = 10
    target_cpu_utilization = 70
  }
  description = "Autoscaling configuration"
}

variable "cluster_endpoint" {
  type        = string
  default     = ""
  description = "Kubernetes cluster endpoint"
}

variable "api_endpoint" {
  type        = string
  default     = ""
  description = "API endpoint"
}

variable "cdn_domain" {
  type        = string
  default     = ""
  description = "CDN domain"
}

output "url" {
  value       = "https://www.example.com"
  description = "Frontend URL"
}
