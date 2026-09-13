variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "random" {
  type    = string
  default = "random"
}

# Use Terraform's built-in provider so the hook test needs no registry downloads.
resource "terraform_data" "this" {
  input = var.random
}

output "random" {
  value = terraform_data.this.output
}
