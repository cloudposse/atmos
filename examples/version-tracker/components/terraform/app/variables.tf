variable "environment" {
  type        = string
  description = "Environment name (dev or prod)"
}

variable "kubectl_version" {
  type        = string
  description = "kubectl version resolved from the Atmos Version Tracker"
}

variable "redis_image" {
  type        = string
  description = "Redis image reference resolved from the Atmos Version Tracker"
}

variable "ci_action_ref" {
  type        = string
  description = "actions/setup-node ref resolved from the Atmos Version Tracker"
}
