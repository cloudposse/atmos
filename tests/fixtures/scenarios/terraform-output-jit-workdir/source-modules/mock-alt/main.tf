variable "stage" {
  type    = string
  default = "test"
}

variable "foo" {
  type    = string
  default = "foo"
}

output "stage" {
  value = var.stage
}

output "foo" {
  value = var.foo
}

# Extra output to prove this came from source-modules/mock-alt, not components/terraform/mock
output "source" {
  value = "hydrated-from-local-source"
}
