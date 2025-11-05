module "label5" {
  source      = "../../"
  enabled     = false
  namespace   = "eg"
  environment = "demo"
  name        = "blue"
  attributes  = ["cluster"]
  delimiter   = "-"

  label_order = ["namespace", "stage", "environment", "attributes"]

  tags = {
  }
}

output "label5" {
  value = {
    id         = module.label5.id
    name       = module.label5.name
    namespace  = module.label5.namespace
    stage      = module.label5.stage
    attributes = module.label5.attributes
    delimiter  = module.label5.delimiter
  }
}

output "label5_tags" {
  value = module.label5.tags
}

output "label5_context" {
  value = module.label5.context
}
