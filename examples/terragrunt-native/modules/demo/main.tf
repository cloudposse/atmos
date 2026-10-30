terraform {
  required_version = ">= 1.4.0"
}

variable "greeting" {
  description = "Message supplied by the Terragrunt unit."
  type        = string
}

# The built-in resource needs no provider download or cloud credentials.
resource "terraform_data" "demo" {
  input = var.greeting
}

output "greeting" {
  value = terraform_data.demo.output
}
