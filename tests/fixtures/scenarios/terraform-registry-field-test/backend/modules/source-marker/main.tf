terraform {
  required_providers {
    null = { source = "hashicorp/null", version = "3.2.4" }
  }
}
variable "value" { type = string }
resource "terraform_data" "marker" { input = "${var.value}-source-v1" }
resource "null_resource" "cache_probe" { triggers = { value = var.value } }
output "value" { value = terraform_data.marker.output }
