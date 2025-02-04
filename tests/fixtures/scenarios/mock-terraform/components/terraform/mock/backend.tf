terraform {
  backend "local" {
    workspace_dir = "terraform.tfstate.d"
  }
}
