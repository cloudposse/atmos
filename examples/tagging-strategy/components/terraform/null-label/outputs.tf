output "id" {
  description = "Generated resource ID."
  value       = join(var.delimiter, local.id_elements)
}

output "tags" {
  description = "Tags after key and value case normalization."
  value       = terraform_data.this.output
}
