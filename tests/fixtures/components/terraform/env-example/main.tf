terraform {
  required_providers {
    environment = {
      source  = "EppO/environment"
      version = "~> 1.3.0" # Check for latest version
    }
  }
}

# Get all environment variables matching patterns
data "environment_variables" "required" {
  filter = "^ATMOS_.*|^EXAMPLE$" # Regex pattern
}
