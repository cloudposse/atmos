terraform {
  required_providers {
    environment = {
      source  = "EppO/environment"
      version = "1.3.8"
    }
  }
}

# Get all environment variables matching patterns.
# Extended to include test patterns for global env testing.
data "environment_variables" "required" {
  filter = "^ATMOS_.*|^EXAMPLE$|^GLOBAL_ENV_.*|^STACK_ENV_.*|^COMPONENT_ENV_.*|^OVERRIDE_ME$"
}
