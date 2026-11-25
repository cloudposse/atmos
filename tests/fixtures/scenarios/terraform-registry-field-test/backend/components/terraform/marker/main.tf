variable "value" { type = string }
variable "inputs" {
  type = any
  default = {}
}
resource "terraform_data" "marker" { input = var.value }
output "value" { value = terraform_data.marker.output }
output "inputs" { value = var.inputs }
