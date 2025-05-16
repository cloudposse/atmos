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

output "object" {
  value = { 
    "bar": var.bar
    "baz": var.baz
  }
}

output "list" {
  value = [var.baz, var.baz]
}
