variable "region" {
  type        = string
  description = "AWS region. Consumed by the AWS provider; supplied by the region mixin."
  default     = ""
}

variable "namespace" {
  type        = string
  description = "Organization namespace (e.g. `acme`)."
  default     = ""
}

variable "tenant" {
  type        = string
  description = "Tenant/OU name (e.g. `plat`)."
  default     = ""
}

variable "environment" {
  type        = string
  description = "Environment/region code (e.g. `ue2`)."
  default     = ""
}

variable "stage" {
  type        = string
  description = "Stage/account name (e.g. `dev`)."
  default     = ""
}

variable "name" {
  type        = string
  description = "Component name, used to build the resource identifier."
}

variable "deletion_window_in_days" {
  type        = number
  description = "Number of days before the KMS key is deleted after destruction."
  default     = 7
}

variable "enable_key_rotation" {
  type        = bool
  description = "Whether to enable automatic annual key rotation."
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to all resources."
  default     = {}
}
