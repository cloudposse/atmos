terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0.0"
    }
  }
}

variable "environment" {
  type = string
  description = "Environment name"
}

variable "stage" {
  type = string
  description = "Stage name"
}

variable "tenant" {
  type = string
  description = "Tenant name"
}

variable "foo" {
  type    = string
  default = "foo"
}

variable "bar" {
  type    = string
  default = "bar"
}

variable "baz" {
  type    = string
  default = "baz"
}

# Add a random string just to ensure the provider is used
resource "random_string" "test" {
  length = 8
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

output "random_string" {
  value = random_string.test.result
}
