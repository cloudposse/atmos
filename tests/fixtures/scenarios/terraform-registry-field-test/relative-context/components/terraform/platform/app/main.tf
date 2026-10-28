terraform {
  required_version = ">= 1.0"
}

module "sibling" {
  source = "../../../../modules/marker"
}

output "sibling" {
  value = module.sibling.value
}

output "template" {
  value = trimspace(file("${path.module}/../../../../templates/message.txt"))
}
