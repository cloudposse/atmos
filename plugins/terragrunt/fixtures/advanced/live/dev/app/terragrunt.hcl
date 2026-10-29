include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
  merge_strategy = "deep"
}

# Order-only edge, separate from the output-bearing database/cache dependencies.
dependencies {
  paths = ["../network"]
}

dependency "database" {
  config_path = "../database"
  mock_outputs = { value = { unit = "database", revision = "mock" } }
  mock_outputs_allowed_terraform_commands = ["plan", "validate"]
}

dependency "cache" {
  config_path = "../cache"
  mock_outputs = { value = { unit = "cache", revision = "mock" } }
  mock_outputs_allowed_terraform_commands = ["plan", "validate"]
}

inputs = {
  payload = {
    unit = "app"
    tags = { app = "storefront" }
    ports = [8080]
    upstream = {
      database = dependency.database.outputs.value
      cache = dependency.cache.outputs.value
    }
  }
}
