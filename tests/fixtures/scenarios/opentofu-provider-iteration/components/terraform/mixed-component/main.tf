# Provider iteration combined with a "Variables not allowed" diagnostic, which Atmos treats as module source
# interpolation. Atmos skips terraform-config-inspect validation, leaving terraform_config nil, and must still
# keep the `sensitive = true` input out of the generated varfile. The pinned terraform-config-inspect accepts
# variables in module source, so the legacy provider `version` argument below raises the diagnostic instead.
variable "accounts" {
  type = list(string)
}

variable "module_name" {
  type = string
}

variable "aws_provider_version" {
  type = string
}

variable "api_token" {
  type      = string
  sensitive = true
}

provider "aws" {
  version  = var.aws_provider_version
  alias    = "by_account"
  for_each = toset(var.accounts)
  profile  = each.value
}

data "aws_caller_identity" "each" {
  for_each = toset(var.accounts)
  provider = aws.by_account[each.value]
}

module "example" {
  source = "./modules/${var.module_name}"
}
