terraform {
  required_providers {
    local = {
      source  = "hashicorp/local"
      version = ">= 2.0.0"
    }
  }
}

variable "environment" {
  type = string
}

variable "stage" {
  type = string
}

variable "tenant" {
  type = string
}

resource "local_file" "test" {
  content  = "test-${var.environment}"
  filename = "./test.txt"
}

variable "foo" {
  type = string
  default = "foo"
}

variable "bar" {
  type = string
  default = "bar"
}

variable "baz" {
  type = string
  default = "baz"
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
