# Transit Gateway Attachment Outputs

output "appliance_mode_support" {
  description = "Whether appliance mode is enabled"
  value       = var.appliance_mode_support
}

output "dns_support" {
  description = "Whether DNS support is enabled"
  value       = var.dns_support
}

output "tags" {
  description = "Resource tags"
  value       = var.tags
}
