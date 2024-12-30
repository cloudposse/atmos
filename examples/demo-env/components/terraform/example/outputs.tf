# Output environment variables
output "atmos_cli_config_path" {
  value       = data.environment_variables.required.items["ATMOS_CLI_CONFIG_PATH"]
  description = "The path to the Atmos CLI configuration file"
}

output "atmos_base_path" {
  value       = data.environment_variables.required.items["ATMOS_BASE_PATH"]
  description = "The base path used by Atmos"
}

output "example" {
  value       = data.environment_variables.required.items["EXAMPLE"]
  description = "Example environment variable"
}
