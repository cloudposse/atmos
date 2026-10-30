# Keep the existing Terragrunt layout and evaluate its inputs natively.
terraform {
  source = "../../modules/demo"
}

remote_state {
  backend = "local"
  config = {
    path = "${get_terragrunt_dir()}/terraform.tfstate"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

inputs = {
  greeting = upper("hello from terragrunt")
}
