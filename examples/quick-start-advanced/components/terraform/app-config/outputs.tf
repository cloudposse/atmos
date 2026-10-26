output "id" {
  description = "The fully-qualified component identifier (namespace-tenant-environment-stage-name)."
  value       = local.id
}

output "parameter_prefix" {
  description = "The SSM Parameter Store path prefix under which all parameters are written."
  value       = local.prefix
}

output "config_parameter_names" {
  description = "The names of the non-secret configuration parameters written to SSM."
  value       = [for k, p in aws_ssm_parameter.config : p.name]
}
