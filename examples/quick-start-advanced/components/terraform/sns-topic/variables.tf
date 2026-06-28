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

variable "kms_key_arn" {
  type        = string
  description = "ARN of the KMS key used to encrypt messages at rest. When empty, encryption is not configured."
  default     = ""
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to all resources."
  default     = {}
}
