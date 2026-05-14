# Secrets masking demo component.
#
# This component outputs various secret values to demonstrate
# the automatic masking feature in Atmos terminal output.

# Create a null resource just to have something to plan/apply.
resource "null_resource" "demo" {
  triggers = {
    demo_api_key   = var.demo_api_key
    internal_id    = var.internal_id
    custom_token   = var.custom_token
    literal_secret = var.literal_secret
    plain_value    = var.plain_value
  }
}

# Output the values so they appear in terraform output.
# These will be masked according to atmos.yaml configuration.
output "demo_api_key" {
  description = "Demo API key (should be masked in output)."
  value       = var.demo_api_key
}

output "internal_id" {
  description = "Internal ID (should be masked in output)."
  value       = var.internal_id
}

output "custom_token" {
  description = "Custom token (should be masked in output)."
  value       = var.custom_token
}

output "literal_secret" {
  description = "Literal secret value (should be masked in output)."
  value       = var.literal_secret
}

output "plain_value" {
  description = "Plain value (should NOT be masked)."
  value       = var.plain_value
}
