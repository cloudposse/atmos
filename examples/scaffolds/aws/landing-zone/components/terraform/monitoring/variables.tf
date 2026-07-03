variable "project" {
  description = "Project name, used in resource names."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "retention_in_days" {
  description = "How long CloudWatch keeps logs in the environment log group."
  type        = number
  default     = 30
}

variable "alarm_threshold_bytes" {
  description = "Log bytes per 5-minute period above which the log-volume alarm fires."
  type        = number
  default     = 536870912
}
