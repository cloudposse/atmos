module "label3c" {
  source              = "../../"
  name                = "Starfish"
  stage               = "release"
  context             = module.label1.context
  delimiter           = "."
  regex_replace_chars = "/[^-a-zA-Z0-9.]/"

  tags = {
    "Eat"    = "Carrot"
    "Animal" = "Rabbit"
  }
}

output "label3c" {
  value = {
    id         = module.label3c.id
    name       = module.label3c.name
    namespace  = module.label3c.namespace
    stage      = module.label3c.stage
    tenant     = module.label3c.tenant
    attributes = module.label3c.attributes
    delimiter  = module.label3c.delimiter
  }
}

output "label3c_tags" {
  value = module.label3c.tags
}

output "label3c_context" {
  value = module.label3c.context
}

output "label3c_normalized_context" {
  value = module.label3c.normalized_context
}
