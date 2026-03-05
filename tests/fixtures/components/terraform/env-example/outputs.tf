output "atmos_cli_config_path" {
  value       = data.external.env.result["atmos_cli_config_path"]
  description = "The path to the Atmos CLI configuration file"
}

output "atmos_base_path" {
  value       = data.external.env.result["atmos_base_path"]
  description = "The base path used by Atmos"
}

output "example" {
  value       = data.external.env.result["example"]
  description = "Example environment variable"
}

# Output all matched variables with uppercase keys for backward compatibility.
output "all_atmos_vars" {
  value = {
    "ATMOS_BASE_PATH"       = data.external.env.result["atmos_base_path"]
    "ATMOS_CLI_CONFIG_PATH" = data.external.env.result["atmos_cli_config_path"]
    "EXAMPLE"               = data.external.env.result["example"]
  }
  description = "All matched environment variables"
}

variable "stage" {
  type        = string
  description = "Deployment stage/environment (e.g., dev, prod)"
}
