module "label4" {
  source      = "../../"
  namespace   = "CloudPosse"
  environment = "UAT"
  name        = "Example Cluster"
  attributes  = ["big", "fat", "honking", "cluster"]
  delimiter   = "-"

  label_order = ["namespace", "stage", "environment", "attributes"]

  tags = {
    "City"        = "Dublin"
    "Environment" = "Private"
  }
}

output "label4" {
  value = {
    id         = module.label4.id
    name       = module.label4.name
    namespace  = module.label4.namespace
    stage      = module.label4.stage
    attributes = module.label4.attributes
    delimiter  = module.label4.delimiter
  }
}

output "label4_tags" {
  value = module.label4.tags
}

output "label4_context" {
  value = module.label4.context
}
