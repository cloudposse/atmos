variable "repository" {
  type        = string
  description = "GitHub repository in owner/repo format"
  default     = "cloudposse/atmos"
}

variable "stage" {
  type        = string
  description = "Stage (environment) name"
  default     = ""
}
