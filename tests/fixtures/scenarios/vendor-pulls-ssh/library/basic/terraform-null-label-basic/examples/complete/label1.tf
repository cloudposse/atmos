module "label1" {
  source      = "../../"
  namespace   = "CloudPosse"
  tenant      = "H.R.H"
  environment = "UAT"
  stage       = "build"
  name        = "Winston Churchroom"
  attributes  = ["fire", "water", "earth", "air"]

  label_order = ["name", "tenant", "environment", "stage", "attributes"]

  tags = {
    "City"        = "Dublin"
    "Environment" = "Private"
  }
}

output "label1" {
  value = {
    id         = module.label1.id
    name       = module.label1.name
    namespace  = module.label1.namespace
    stage      = module.label1.stage
    tenant     = module.label1.tenant
    attributes = module.label1.attributes
    delimiter  = module.label1.delimiter
  }
}

output "label1_tags" {
  value = module.label1.tags
}

output "label1_context" {
  value = module.label1.context
}

output "label1_normalized_context" {
  value = module.label1.normalized_context
}



