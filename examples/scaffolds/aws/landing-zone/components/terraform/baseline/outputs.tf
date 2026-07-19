output "parameter_names" {
  description = "Names of the published SSM parameters."
  value       = [for p in aws_ssm_parameter.metadata : p.name]
}
