# API Service Component
# Stub component for demo purposes

variable "enabled" {
  type        = bool
  default     = true
  description = "Enable the API service"
}

variable "name" {
  type        = string
  description = "Name of the API service"
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

variable "cluster_endpoint" {
  type        = string
  default     = ""
  description = "Kubernetes cluster endpoint"
}

variable "database_endpoint" {
  type        = string
  default     = ""
  description = "Database endpoint"
}

# Variables passed from stack configuration (catalog).

variable "cpu_request" {
  type        = string
  description = "CPU request."
  default     = "256m"
}

variable "memory_request" {
  type        = string
  description = "Memory request."
  default     = "512Mi"
}

variable "cpu_limit" {
  type        = string
  description = "CPU limit."
  default     = "1000m"
}

variable "memory_limit" {
  type        = string
  description = "Memory limit."
  default     = "2Gi"
}

variable "health_check_path" {
  type        = string
  description = "Health check path."
  default     = "/health"
}

variable "metrics_enabled" {
  type        = bool
  description = "Enable metrics."
  default     = true
}

variable "autoscaling" {
  type        = any
  description = "Autoscaling configuration."
  default     = {}
}

output "service_url" {
  value       = "https://api.example.com"
  description = "API service URL"
}

output "endpoint" {
  value       = "https://api.example.com/v1"
  description = "API endpoint"
}
