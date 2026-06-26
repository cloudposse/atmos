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

variable "hash_key" {
  type        = string
  description = "Name of the table's partition (hash) key attribute."
  default     = "id"
}

variable "billing_mode" {
  type        = string
  description = "Controls how the table is charged for reads and writes (`PAY_PER_REQUEST` or `PROVISIONED`)."
  default     = "PAY_PER_REQUEST"
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to all resources."
  default     = {}
}
