variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "random" {
  type    = string
  default = "random"
}

resource "null_resource" "this" {
  # Changes to any instance of the cluster requires re-provisioning
  triggers = {
    random = var.random
  }
}

output "random" {
  value = null_resource.this.triggers.random
}
