locals {
  environment = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  revision = get_env("POC_REVISION", "r1")
}

terraform {
  source = "${dirname(find_in_parent_folders("root.hcl"))}/modules/unit"
}

remote_state {
  backend = "local"
  config = { path = "${get_terragrunt_dir()}/state/terraform.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}

generate "context" {
  path = "context.tf"
  if_exists = "overwrite"
  disable_signature = true
  contents = "output \"generated_region\" { value = \"${local.environment.locals.region}\" }\n"
}

inputs = {
  payload = {
    region = local.environment.locals.region
    tags = local.environment.locals.tags
    ports = [80]
    revision = local.revision
    origin = path_relative_to_include()
    upstream = {}
  }
}
