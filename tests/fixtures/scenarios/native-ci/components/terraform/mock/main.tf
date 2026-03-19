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

resource "null_resource" "test" {
  triggers = {
    test = "test"
  }

  provisioner "local-exec" {
    command = "exit 0"
  }
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
