output "stage" {
  description = "Stage of deployment"
  value       = var.stage
}

output "parameter_name" {
  description = "Name of the SSM parameter created in the local AWS emulator"
  value       = aws_ssm_parameter.marker.name
}
