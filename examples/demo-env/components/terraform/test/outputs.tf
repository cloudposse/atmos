# Output all environment variables
# Define variables with fallback values if not set
output "atmos_cli_config_path" {
  value       = data.environment_variables.required.items["ATMOS_CLI_CONFIG_PATH"]
  description = "The path to the Atmos CLI configuration file"
}

output "atmos_base_path" {
  value       = data.environment_variables.required.items["ATMOS_BASE_PATH"]
  description = "The base path used by Atmos"
}
output "stage" {
  value       = var.stage
  description = "Stage where it was deployed"
}
variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}
