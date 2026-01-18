# This component produces warning diagnostics via check blocks.
# Check blocks (Terraform 1.5+) produce warnings that don't block execution.

terraform {
  required_version = ">= 1.5.0"
}

variable "enabled" {
  type        = bool
  default     = true
  description = "Whether to enable this component"
}

variable "warning_threshold" {
  type        = number
  default     = 100
  description = "Threshold value that will be checked (default triggers warning)"
}

# This check block will produce a WARNING during plan/apply.
# Check block assertions that fail produce warnings, not errors.
check "threshold_warning" {
  assert {
    condition     = var.warning_threshold < 50
    error_message = "Test warning: warning_threshold (${var.warning_threshold}) exceeds safe limit of 50"
  }
}

output "status" {
  value = "diagnostic-warning component executed"
}

output "threshold" {
  value = var.warning_threshold
}
