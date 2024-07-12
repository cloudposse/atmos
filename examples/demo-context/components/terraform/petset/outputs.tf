output "size" {
    description = "Size of deployment"
    value = var.size
}

output "tags" {
  description = "Generated Resource Tags"
  value = data.context_tags.this.tags
}

output "label" {
    description = "Generated Resource Label"
    value = data.context_label.this.rendered
}

output "pet_set" {
  value = [for k, v in module.pet_set : v.name]
}

output "delimiter" {
    description = "Delimiter used in the context"
    value = data.context_config.this.delimiter
}
