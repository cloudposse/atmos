# 🎲 Random Component for Testing
# This component is designed to be simple, reusable, and perfect for testing Atmos functionality

terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = ">= 2.0"
    }
  }
}

# Common variables used across test scenarios
variable "stage" {
  type        = string
  description = "Stage (e.g., dev, staging, prod)"
}

variable "environment" {
  type        = string
  description = "Environment (e.g., ue2, uw2)"
}

variable "tenant" {
  type        = string
  description = "Tenant name"
}

# Test-specific variables with defaults
variable "foo" {
  type        = string
  default     = "foo"
  description = "Test variable foo"
}

variable "bar" {
  type        = string
  default     = "bar"
  description = "Test variable bar"
}

variable "baz" {
  type        = string
  default     = "baz"
  description = "Test variable baz"
}

# Simple local file resource for testing
resource "local_file" "test" {
  content = jsonencode({
    metadata = {
      stage       = var.stage
      environment = var.environment
      tenant      = var.tenant
      timestamp   = timestamp()
    }
    config = {
      foo = var.foo
      bar = var.bar
      baz = var.baz
    }
  })
  filename = "${path.module}/test-${var.environment}-${var.stage}.json"
}

# Outputs for testing
output "stage" {
  value = var.stage
}

output "environment" {
  value = var.environment
}

output "tenant" {
  value = var.tenant
}

output "foo" {
  value = var.foo
}

output "bar" {
  value = var.bar
}

output "baz" {
  value = var.baz
}

output "file_path" {
  value = local_file.test.filename
}

output "file_content" {
  value = local_file.test.content
}
