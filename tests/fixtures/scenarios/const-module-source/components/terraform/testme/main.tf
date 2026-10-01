# This component uses an interpolated `module.source`, which is valid Terraform 1.15+
# (and OpenTofu 1.8+) because `var.org` is a static variable (`const = true`).
#
# Older versions of `terraform-config-inspect` decoded `module.source` with a nil
# `hcl.EvalContext` and emitted a "Variables not allowed" diagnostic, which Atmos treated as
# fatal. hashicorp/terraform-config-inspect#146 ("Tolerate const vars and locals in module
# source and version") fixed that upstream.
#
# The module source is a LOCAL path so no registry, credentials or network are needed.

variable "org" {
  const   = true
  type    = string
  default = "acme"
}

variable "enabled" {
  type    = bool
  default = true
}

variable "name" {
  type    = string
  default = "default-name"
}

module "greeting" {
  source = "./mods/${var.org}"

  name = var.name
}

output "greeting" {
  value = module.greeting.greeting
}
