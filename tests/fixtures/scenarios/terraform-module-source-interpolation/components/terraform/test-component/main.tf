# Test component reproducing issue #2913: Terraform 1.15+ allows variable
# interpolation in a module's `source` attribute when the variable is
# declared `const = true`. This is valid HCL under plain Terraform 1.15+
# (no OpenTofu required), but Atmos's terraform-config-inspect-based
# validation historically only tolerated this pattern for OpenTofu.
# See: https://github.com/cloudposse/atmos/issues/2913

variable "org" {
  description = "Organization name used to select the module directory"
  const       = true
  type        = string
  default     = "acme"
}

module "greeting" {
  source = "./mods/${var.org}"
}

output "org_value" {
  description = "Output to verify the variable is available"
  value       = var.org
}
