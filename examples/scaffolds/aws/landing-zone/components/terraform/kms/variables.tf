variable "project" {
  description = "Project name, used in resource names."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "deletion_window_in_days" {
  description = "Waiting period before a scheduled key deletion becomes final."
  type        = number
  default     = 7
}
