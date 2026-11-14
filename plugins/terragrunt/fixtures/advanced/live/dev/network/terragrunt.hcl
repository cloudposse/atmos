include "root" {
  path = find_in_parent_folders("root.hcl")
  expose = true
  merge_strategy = "deep"
}

inputs = {
  payload = {
    unit = "network"
    tags = { stage = get_env("POC_ENV", "dev") }
    ports = [443]
    exposed_region = include.root.locals.environment.locals.region
  }
}
