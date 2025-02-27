module "label8l" {
  source           = "../../"
  enabled          = true
  namespace        = "eg"
  environment      = "demo"
  name             = "blue"
  attributes       = ["cluster"]
  delimiter        = "-"
  label_key_case   = "lower"
  label_value_case = "lower"

  tags = {
    "kubernetes.io/cluster/" = "shared"
    "upperTEST"              = "testUPPER"
  }
}

module "label8l_context" {
  source = "../../"

  context = module.label8l.context
}

output "label8l_context_id" {
  value = module.label8l_context.id
}

output "label8l_context_context" {
  value = module.label8l_context.context
}

output "label8l_context_tags" {
  value = module.label8l_context.tags
}

output "label8l_id" {
  value = module.label8l.id
}

output "label8l_context" {
  value = module.label8l.context
}

output "label8l_tags" {
  value = module.label8l.tags
}
