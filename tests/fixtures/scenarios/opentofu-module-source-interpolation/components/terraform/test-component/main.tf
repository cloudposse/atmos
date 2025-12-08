# Test component that uses variable interpolation in module source
# This feature requires OpenTofu 1.8+ or Terraform with experimental features
# See: https://github.com/cloudposse/atmos/issues/1753

variable "context" {
  description = "Nested variable structure for module configuration"
  type = object({
    build = object({
      module_path    = string
      module_version = string
    })
  })
}

variable "simple_var" {
  description = "Simple flat variable for comparison"
  type        = string
  default     = ""
}

variable "another_var" {
  description = "Another flat variable"
  type        = string
  default     = ""
}

# This is the problematic line reported in issue #1753
# OpenTofu 1.8+ supports variable interpolation in module source
module "themodule" {
  source = var.context.build.module_path
  # Pass through some variables to verify they're available
  test_input = var.simple_var
}

output "context_value" {
  description = "Output to verify nested variable is available"
  value       = var.context
}

output "simple_var_value" {
  description = "Output to verify flat variable is available"
  value       = var.simple_var
}
