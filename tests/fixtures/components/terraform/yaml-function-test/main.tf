# yaml-function-test component
# A generic test component for validating YAML function behavior
# This component doesn't create any real infrastructure

terraform {
  required_version = ">= 1.0"
}

# Local values for internal testing
locals {
  # These can be used to simulate various outputs
  simulated_string = "simulated-${var.test_string}"
  simulated_list   = concat(["simulated"], var.string_list)
  simulated_map    = merge({ simulated = "value" }, var.string_map)
}
