module "label1t1" {
  source = "../../"

  id_length_limit = 32

  context = module.label1.context
}

output "label1t1" {
  value = {
    id      = module.label1t1.id
    id_full = module.label1t1.id_full
  }
}

output "label1t1_tags" {
  value = module.label1t1.tags
}