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

variable "kms_key_arn" {
  description = "ARN of a KMS key used to encrypt the log group. Empty string leaves logs encrypted with the default CloudWatch Logs service key."
  type        = string
  default     = ""
}
