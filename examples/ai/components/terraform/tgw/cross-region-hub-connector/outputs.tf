# Transit Gateway Cross-Region Hub Connector Outputs

output "peer_region" {
  description = "Peer region for the cross-region connection"
  value       = var.peer_region
}

output "auto_accept_peering" {
  description = "Whether peering is auto-accepted"
  value       = var.auto_accept_peering
}

output "tags" {
  description = "Resource tags"
  value       = var.tags
}
