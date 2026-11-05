terraform {
  required_version = ">= 1.4.0"
}

variable "log_retention_days" {
  type        = number
  description = "Selected log retention period in days."
}

# Display the selected configuration without creating cloud resources.
resource "terraform_data" "this" {
  input = {
    log_retention_days = var.log_retention_days
  }
}
