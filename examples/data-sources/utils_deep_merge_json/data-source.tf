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
  json_data_1 = file("${path.module}/json1.json")
  json_data_2 = file("${path.module}/json2.json")
}

data "utils_deep_merge_json" "example" {
  input = [
    local.json_data_1,
    local.json_data_2
  ]
}

output "deep_merge_output" {
  value = jsondecode(data.utils_deep_merge_json.example.output)
}
