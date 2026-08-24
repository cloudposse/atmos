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

variable "topic_arn" {
  type        = string
  description = "ARN of an SNS topic to subscribe this queue to. When empty, no subscription or access policy is created."
  default     = ""
}

variable "visibility_timeout_seconds" {
  type        = number
  description = "Time (in seconds) a received message is hidden from subsequent retrieve requests."
  default     = 30
}

variable "message_retention_seconds" {
  type        = number
  description = "Number of seconds a message is retained in the queue."
  default     = 345600
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to all resources."
  default     = {}
}
