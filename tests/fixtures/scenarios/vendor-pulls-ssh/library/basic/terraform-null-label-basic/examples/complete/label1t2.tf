module "label1t2" {
  source = "../../"

  id_length_limit = 33

  context = module.label1.context
}

output "label1t2" {
  value = {
    id      = module.label1t2.id
    id_full = module.label1t2.id_full
  }
}

output "label1t2_tags" {
  value = module.label1t2.tags
}