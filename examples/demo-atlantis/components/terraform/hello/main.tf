terraform {
  backend "local" {}
}

variable "stage" {
  type = string
}

variable "greeting" {
  type = string
}

output "message" {
  value = "${var.greeting} (${var.stage})"
}
