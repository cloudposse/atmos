locals {
  project_root = dirname(get_terragrunt_dir())
}

stack "dev" {
  source = "../catalog/stacks/environment"
  path = "dev"
  values = {
    project_root = local.project_root
    stage = "dev"
    revision = get_env("POC_REVISION", "r1")
  }
}

stack "prod" {
  source = "../catalog/stacks/environment"
  path = "prod"
  values = {
    project_root = local.project_root
    stage = "prod"
    revision = get_env("POC_REVISION", "r1")
  }
}
