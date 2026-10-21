# This mock mirrors the input variables of the `cloudposse/label/null` module so the
# example runs without downloading anything. Real components would not declare these
# variables by hand. They would vendor `context.tf`, which declares the same inputs and
# instantiates `module "this" { source = "cloudposse/label/null" ... }`.

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

variable "delimiter" {
  type        = string
  description = "Delimiter between ID elements."
  default     = "-"
}

variable "tags" {
  type        = map(string)
  description = "Tags merged across all inheritance levels by Atmos."
  default     = {}
}

variable "label_key_case" {
  type        = string
  description = "Case of tag keys: lower, title, or upper. Anything else leaves keys unchanged."
  default     = null
}

variable "label_value_case" {
  type        = string
  description = "Case of tag values: lower, title, or upper. Anything else leaves values unchanged."
  default     = null
}
