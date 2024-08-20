output "network_id_1" {
  value       = one(google_compute_network.network_1[*].id)
  description = "Network ID 1"
}

output "network_id_2" {
  value       = one(google_compute_network.network_2[*].id)
  description = "Network ID 2"
}

output "subnets_1" {
  value       = google_compute_subnetwork.subnets_1
  description = "Map of created subnets in the network 1"
}

output "subnets_2" {
  value       = google_compute_subnetwork.subnets_2
  description = "Map of created subnets in the network 2"
}
