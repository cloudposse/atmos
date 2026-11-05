terraform {
  required_version = ">= 1.4.0"
}

variable "audit_logging_enabled" {
  type        = bool
  description = "Whether the stack selects audit logging."
}

# Display the selected configuration without creating cloud resources.
resource "terraform_data" "this" {
  input = {
    audit_logging_enabled = var.audit_logging_enabled
  }
}
