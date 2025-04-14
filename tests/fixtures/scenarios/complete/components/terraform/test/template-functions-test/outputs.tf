output "test_label_id" {
  value       = module.test_label.id
  description = "Test label ID"
}

output "test_list" {
  value = [
    "list_item_1",
    "list_item_2",
    "list_item_3"
  ]
  description = "Test list"
}

output "test_map" {
  value = {
    a = 1,
    b = 2,
    c = 3
  }
  description = "Test map"
}

output "tags" {
  value       = module.test_label.tags
  description = "Tags"
}
