variable "namespace" {
  type        = string
  description = "Organization name."
}

variable "stage" {
  type        = string
  description = "Stage, such as dev or prod."
}

variable "name" {
  type        = string
  description = "Application name."
}

variable "tags" {
  type        = map(string)
  description = "Tags inherited from stack configuration."
}

module "this" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  namespace = var.namespace
  stage     = var.stage
  name      = var.name
  tags      = var.tags
}
