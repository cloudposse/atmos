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

resource "local_file" "mock" {
  content  = jsonencode({
    foo = var.foo
    bar = var.bar
    baz = var.baz
  })
  filename = "${path.module}/mock.json"
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
