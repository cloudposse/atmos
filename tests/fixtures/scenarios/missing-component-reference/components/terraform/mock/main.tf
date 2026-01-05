variable "foo" {
  type    = string
  default = ""
}

output "foo" {
  value = var.foo
}
