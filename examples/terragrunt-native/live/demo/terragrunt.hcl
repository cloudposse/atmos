terraform {
  source = "${get_terragrunt_dir()}/../../modules/demo"
}

remote_state {
  backend = "local"
  config = {
    path = "${get_terragrunt_dir()}/state/terraform.tfstate"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  greeting = upper("hello from terragrunt")
}
