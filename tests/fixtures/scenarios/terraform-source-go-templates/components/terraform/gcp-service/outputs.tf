output "service_id" {
  value       = local.service_id
  description = "Unique Identifier for the created service with format projects/{{project}}/locations/{{location}}/services/{{name}}"
}
