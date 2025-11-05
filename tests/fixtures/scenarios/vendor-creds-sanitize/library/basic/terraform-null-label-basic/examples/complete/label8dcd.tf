module "label8dcd" {
  source = "../../"

  enabled     = true
  namespace   = "eg"
  environment = "demo"
  name        = "blue"
  attributes  = ["cluster"]
  delimiter   = "x"
}

module "label8dcd_context" {
  source = "../../"

  context = module.label8dcd.context
}

output "label8dcd_context_id" {
  value = module.label8dcd_context.id
}

output "label8dcd_id" {
  value = module.label8dcd.id
}
