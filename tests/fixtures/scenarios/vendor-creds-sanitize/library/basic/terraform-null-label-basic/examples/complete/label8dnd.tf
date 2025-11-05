module "label8dnd" {
  source = "../../"

  enabled     = true
  namespace   = "eg"
  environment = "demo"
  name        = "blue"
  attributes  = ["cluster"]
  delimiter   = ""
}

module "label8dnd_context" {
  source = "../../"

  context = module.label8dnd.context
}

output "label8dnd_context_id" {
  value = module.label8dnd_context.id
}

output "label8dnd_id" {
  value = module.label8dnd.id
}
