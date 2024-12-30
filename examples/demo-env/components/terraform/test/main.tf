terraform {
  required_providers {
    environment = {
      source  = "EppO/environment"
      version = ">= 1.0.0"
    }
  }
}

# Provider declaration for the hypothetical environment provider
provider "environment" {}


# Fetch the required environment variables using the `environment_variables` data source
data "environment_variables" "required" {
  filter = "ATMOS_.*" # Fetches all variables starting with "ATMOS_"
}
