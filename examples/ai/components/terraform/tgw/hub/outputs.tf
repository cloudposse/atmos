# Transit Gateway Hub Outputs

output "amazon_side_asn" {
  description = "ASN of the Transit Gateway"
  value       = var.amazon_side_asn
}

output "auto_accept_shared_attachments" {
  description = "Whether shared attachments are auto-accepted"
  value       = var.auto_accept_shared_attachments
}

output "tags" {
  description = "Resource tags"
  value       = var.tags
}
