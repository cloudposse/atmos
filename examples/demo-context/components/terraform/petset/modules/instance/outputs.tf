output "tags" {
  description = "Generated Resource Tags"
  value = data.context_tags.this.tags
}

output "instance_type" {
  value = local.instance_type
}

output "name" {
  value = format("%s %s", local.instance_type, data.context_label.this.rendered)
}
