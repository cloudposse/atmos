variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

terraform {
  required_version = ">= 0.12"
}

# Invalid resource block with a missing argument to cause terraform plan to fail
resource "null_resource" "example" {
  provisioner "local-exec" {
    command = "echo ${var.stage}"
  }
}
