variable "project" {
  description = "Application/project name, used in resource names."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "region" {
  description = "AWS region."
  type        = string
}

variable "force_destroy" {
  description = "Allow destroying the app bucket even when it contains objects."
  type        = bool
  default     = false
}

variable "queue_visibility_timeout_seconds" {
  description = "Visibility timeout for the app work queue."
  type        = number
  default     = 30
}

variable "parameters" {
  description = "Application metadata written under /<project>/<stage>/app/ in SSM Parameter Store."
  type        = map(string)
  default     = {}
}
