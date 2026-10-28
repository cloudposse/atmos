terraform {
  required_version = ">= 1.4.0"
  backend "local" {}
}

variable "name" { type = string }
variable "stage" { type = string }
variable "upstream" {
  type    = string
  default = ""
}

resource "terraform_data" "marker" {
  input = {
    name     = var.name
    upstream = var.upstream
  }

  provisioner "local-exec" {
    command = "echo 'create ${self.input.name}' >> \"$ORDER_LOG\""
  }

  provisioner "local-exec" {
    when    = destroy
    command = "echo 'destroy ${self.input.name}' >> \"$ORDER_LOG\""
  }
}

output "name" { value = terraform_data.marker.output.name }
output "upstream" { value = terraform_data.marker.output.upstream }
