terraform {
  required_providers {
    null = { source = "hashicorp/null", version = "3.2.4" }
  }
}
variable "value" { type = string }
variable "stage" { type = string }
resource "null_resource" "example" { triggers = { value = var.value } }
output "value" { value = var.value }
