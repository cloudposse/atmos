include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
  merge_strategy = "deep"
}

dependency "network" {
  config_path = "../network"
  mock_outputs = { value = { unit = "network", revision = "mock" } }
  mock_outputs_allowed_terraform_commands = ["plan", "validate"]
}

inputs = {
  payload = {
    unit = "cache"
    ports = [6379]
    upstream = { network = dependency.network.outputs.value }
  }
}
