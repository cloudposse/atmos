# This component produces error diagnostics via precondition.
# Preconditions that fail produce errors that block execution.

terraform {
  required_version = ">= 1.0.0"
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
  }
}

variable "enabled" {
  type        = bool
  default     = true
  description = "Whether to enable this component"
}

variable "trigger_error" {
  type        = bool
  default     = true
  description = "When true, the precondition will fail and produce an error diagnostic"
}

# This null_resource has a precondition that fails when trigger_error is true.
# Precondition failures produce error diagnostics.
resource "null_resource" "test" {
  lifecycle {
    precondition {
      condition     = var.trigger_error == false
      error_message = "Test error: Precondition failed because trigger_error is true"
    }
  }
}

output "status" {
  value = "diagnostic-error component executed"
}
