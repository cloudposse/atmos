# This is the default providers.tf when account map is disabled.

variable "account_map_enabled" {
  type        = bool
  description = "Enable the account map component"
  default     = false
}

variable "account_map" {
  type = object({
    full_account_map              = map(string)
    audit_account_account_name    = optional(string, "")
    root_account_account_name     = optional(string, "")
    identity_account_account_name = optional(string, "")
    aws_partition                 = optional(string, "aws")
    iam_role_arn_templates        = optional(map(string), {})
  })
  description = "Map of account names (tenant-stage format) to account IDs. Used to verify we're targeting the correct AWS account. Optional attributes support component-specific functionality (e.g., audit_account_account_name for cloudtrail, root_account_account_name for aws-sso)."
  default = {
    full_account_map              = {}
    audit_account_account_name    = ""
    root_account_account_name     = ""
    identity_account_account_name = ""
    aws_partition                 = "aws"
    iam_role_arn_templates        = {}
  }
}

provider "aws" {
  region = var.region
}

# dummy module to satisfy the module dependency
module "iam_roles" {
  source  = "cloudposse/label/null"
  context = module.this.context
}
