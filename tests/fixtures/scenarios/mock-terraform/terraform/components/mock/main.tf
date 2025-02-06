terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = ">= 2.0"
    }
  }
}

variable "environment" {
  type = string
}

# Mock resource for testing
resource "local_file" "test" {
  content  = "test-${var.environment}"
  filename = "${path.module}/test.txt"
} 