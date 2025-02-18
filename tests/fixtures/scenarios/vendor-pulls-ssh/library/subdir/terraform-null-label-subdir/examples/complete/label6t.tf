module "label6t" {
  source = "../../"

  # Use values from tfvars,
  # specifically: complete.auto.tfvars
  context = module.this.context
}

output "label6t" {
  value = {
    id              = module.label6t.id
    id_full         = module.label6t.id_full
    id_length_limit = module.this.context.id_length_limit
  }
}

output "label6t_tags" {
  value = module.label6t.tags
}