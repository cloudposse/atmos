# OpenTofu 1.9+ provider iteration: one provider configuration per account, selected by key.
# terraform-config-inspect reports `provider = aws.by_account[each.value]` as
# "Invalid provider reference", which Atmos must treat as a known static-parser limitation.
variable "accounts" {
  type = list(string)
}

provider "aws" {
  alias    = "by_account"
  for_each = toset(var.accounts)
  profile  = each.value
}

data "aws_caller_identity" "each" {
  for_each = toset(var.accounts)
  provider = aws.by_account[each.value]
}

module "example" {
  source = "./modules/example"
}
