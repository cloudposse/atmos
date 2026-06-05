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
