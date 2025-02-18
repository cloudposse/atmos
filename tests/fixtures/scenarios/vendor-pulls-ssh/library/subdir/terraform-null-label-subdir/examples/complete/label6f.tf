module "label6f" {
  source = "../../"

  delimiter       = "~"
  id_length_limit = 0

  # Use values from tfvars
  context = module.this.context
}

output "label6f" {
  value = {
    id      = module.label6f.id
    id_full = module.label6f.id_full
  }
}

output "label6f_tags" {
  value = module.label6f.tags
}