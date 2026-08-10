# Reproduces a genuine, unrelated HCL error that co-occurs with a legitimate
# module-source-interpolation diagnostic in the same module. The module block
# (whose "Variables not allowed" diagnostic is known-safe, see
# isKnownModuleSourceInterpolationDiagnostic) is declared BEFORE the output
# block with the real error, so the real error is not diags[0].
#
# Before the fix, terraform-config-inspect's Diagnostics.Error() only renders
# diags[0]'s text, so this genuine error never appeared in the string Atmos
# pattern-matched against -- yet the match still succeeded (diags[0] was the
# module-source diagnostic) and Atmos silently discarded the whole diagnostics
# set, including this real error. See issue #2913 field-test findings.

variable "org" {
  description = "Organization name used to select the module directory"
  const       = true
  type        = string
  default     = "acme"
}

module "greeting" {
  source = "./mods/${var.org}"
}

# Genuine error: "sensitive" must be a bool, not a list. Unrelated to module
# source interpolation, and must never be silently swallowed.
output "org_value" {
  description = "Output to verify the variable is available"
  value       = var.org
  sensitive   = [1, 2, 3]
}
