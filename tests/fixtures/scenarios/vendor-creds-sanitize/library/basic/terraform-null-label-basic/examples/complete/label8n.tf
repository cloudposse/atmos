module "label8n" {
  source = "../../"

  enabled          = true
  namespace        = "EG"
  environment      = "demo"
  name             = "blue"
  attributes       = ["eks", "ClusteR"]
  delimiter        = "-"
  label_value_case = "none"

  tags = {
    "kubernetes.io/cluster/" = "shared"
  }
}

module "label8n_context" {
  source = "../../"

  context = module.label8n.context
}

output "label8n_context_id" {
  value = module.label8n_context.id
}

output "label8n_context_context" {
  value = module.label8n_context.context
}

output "label8n_context_tags" {
  value = module.label8n_context.tags
}

output "label8n_id" {
  value = module.label8n.id
}

output "label8n_context" {
  value = module.label8n.context
}

output "label8n_tags" {
  value = module.label8n.tags
}
