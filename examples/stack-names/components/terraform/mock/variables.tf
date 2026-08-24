variable "environment" {
  description = "The environment name"
  type        = string
}

variable "enabled" {
  description = "Whether this component is enabled"
  type        = bool
  default     = true
}
