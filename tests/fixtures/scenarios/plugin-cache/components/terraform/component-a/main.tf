terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
  }
}

resource "null_resource" "example" {
  triggers = {
    value = var.value
  }
}

variable "value" {
  type    = string
  default = "component-a"
}

output "value" {
  value = var.value
}
