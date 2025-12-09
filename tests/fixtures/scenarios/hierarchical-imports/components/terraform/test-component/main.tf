# Simple terraform component for testing

variable "import_order_test" {
  type        = string
  description = "Test variable to validate import order"
}

variable "region" {
  type        = string
  description = "AWS region"
}

variable "stage" {
  type        = string
  description = "Stage/environment"
}

output "import_order_test" {
  value = var.import_order_test
}

output "region" {
  value = var.region
}

output "stage" {
  value = var.stage
}
