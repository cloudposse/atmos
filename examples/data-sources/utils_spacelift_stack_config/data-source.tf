terraform {
  required_providers {
    utils = {
      source = "cloudposse/utils"
      # For local development,
      # install the provider on local computer by running `make install` from the root of the repo,
      # and uncomment the version below
      # version = "9999.99.99"
    }
  }
}

locals {
  config_filenames   = fileset("./stacks", "*.yaml")
  stack_config_files = [for f in local.config_filenames : format("stacks/%s", f) if(replace(f, "globals", "") == f)]
}

data "utils_spacelift_stack_config" "example" {
  input                      = local.stack_config_files
  process_stack_deps         = false
  process_component_deps     = true
  process_imports            = true
  stack_config_path_template = "stacks/%s.yaml"
}

locals {
  result = yamldecode(data.utils_spacelift_stack_config.example.output)
}

output "output" {
  value = local.result
}
