variable "stage" { type = string }
variable "name" { type = string }
variable "upstream" {
  type = string
  default = ""
}
resource "terraform_data" "marker" {
  input = { name = var.name, upstream = var.upstream, source = "v1" }
}
output "name" { value = terraform_data.marker.output.name }
output "upstream" { value = terraform_data.marker.output.upstream }
output "source" { value = terraform_data.marker.output.source }
