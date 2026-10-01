variable "project" {
  description = "Project name, used in resource names."
  type        = string
}

variable "gcp_project" {
  description = "GCP project ID used by the provider."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "region" {
  description = "GCP region/location."
  type        = string
}

variable "force_destroy" {
  description = "Allow destroying the bucket even when it contains objects."
  type        = bool
  default     = false
}

variable "secret_payload" {
  description = "Demo secret payload written to Secret Manager."
  type        = string
  sensitive   = true
}
