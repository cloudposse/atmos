# A minimal version of the `context.tf` file that Cloud Posse components share. It declares the
# label context inputs and instantiates the Null Label module as `module.this`. The module is a
# child module that every component calls. It is never a component itself.

variable "namespace" {
  type        = string
  description = "Organization identifier, usually the company or product name."
  default     = null
}

variable "tenant" {
  type        = string
  description = "Team or business unit that owns the resource."
  default     = null
}

variable "environment" {
  type        = string
  description = "Environment identifier, such as a region abbreviation."
  default     = null
}

variable "stage" {
  type        = string
  description = "Stage identifier, such as dev or prod."
  default     = null
}

variable "name" {
  type        = string
  description = "Solution name, such as app or cluster."
  default     = null
}

variable "attributes" {
  type        = list(string)
  description = "Additional attributes appended to the generated ID."
  default     = []
}

variable "tags" {
  type        = map(string)
  description = "Tags merged across all inheritance levels by Atmos. Passed through unchanged."
  default     = {}
}

variable "label_key_case" {
  type        = string
  description = "Case of the tag keys the module generates: lower, title, or upper. Defaults to title. Does not change keys in `tags`."
  default     = null
}

variable "label_value_case" {
  type        = string
  description = "Case of the tag values the module generates: lower, title, or upper. Defaults to lower. Does not change values in `tags`."
  default     = null
}

module "this" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  namespace        = var.namespace
  tenant           = var.tenant
  environment      = var.environment
  stage            = var.stage
  name             = var.name
  attributes       = var.attributes
  tags             = var.tags
  label_key_case   = var.label_key_case
  label_value_case = var.label_value_case
}
