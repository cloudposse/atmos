variable "name" {
  type        = string
  description = "Base name for the resources."
}

variable "environment" {
  type        = string
  description = "Environment suffix appended to resource names."
  default     = "test"
}

variable "enable_versioning" {
  type        = bool
  description = "Whether to enable S3 bucket versioning."
  default     = true
}
