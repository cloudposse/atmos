variable "name" { type = string }
variable "stage" { type = string }
module "label" {
  source = "cloudposse/label/null"
  version = "0.25.0"
  name = var.name
  stage = var.stage
}
output "id" { value = module.label.id }
