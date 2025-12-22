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

output "service_url" {
  value       = "https://api.example.com"
  description = "API service URL"
}

output "endpoint" {
  value       = "https://api.example.com/v1"
  description = "API endpoint"
}
