variable "name" {
  type = string
}

variable "upstream" {
  type    = string
  default = ""
}

resource "terraform_data" "marker" {
  input = var.name

  provisioner "local-exec" {
    command = "echo \"terraform apply ${self.input}\" >> \"$ORDER_LOG\""
  }

  provisioner "local-exec" {
    when    = destroy
    command = "echo \"terraform destroy ${self.input}\" >> \"$ORDER_LOG\""
  }
}

output "name" {
  value = var.name
}

output "upstream" {
  value = var.upstream
}
