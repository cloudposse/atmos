# terraform-null-label example used here: Output list of tags applied in each format
output "tags_as_list_of_maps" {
  value = module.label.tags_as_list_of_maps
}

output "tags" {
  value = module.label.tags
}

output "id" {
  value = module.label.id
}
