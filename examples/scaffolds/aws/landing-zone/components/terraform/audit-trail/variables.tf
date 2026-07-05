variable "project" {
  description = "Project name, used in resource names."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "force_destroy" {
  description = "Allow destroying the log bucket even when it contains objects."
  type        = bool
  default     = false
}
