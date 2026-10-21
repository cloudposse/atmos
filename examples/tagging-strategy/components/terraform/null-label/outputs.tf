output "id" {
  description = "Generated resource ID."
  value       = local.id
}

output "tags" {
  description = "Generated label tags merged with the supplied tags."
  value       = local.tags
}
