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

output "foo" {
  value = var.foo
}

output "bar" {
  value = var.bar
}

output "baz" {
  value = var.baz
}
