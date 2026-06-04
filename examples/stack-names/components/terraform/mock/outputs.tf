output "environment" {
  description = "The environment name"
  value       = var.environment
}

output "message" {
  description = "A greeting message"
  value       = local.message
}
