terraform {
  required_version = ">= 1.0"

  # Retained from the Fogg-generated file: the guide keeps the original backend in
  # Terraform while components.terraform.auto_generate_backend_file stays false.
  backend "s3" {
    bucket               = "acme-terraform-state"
    key                  = "terraform/acme/envs/development/components/webserver.tfstate"
    region               = "us-west-2"
    profile              = "acme-state"
    encrypt              = true
    dynamodb_table       = "acme-terraform-locks"
    workspace_key_prefix = "env:"
  }
}
