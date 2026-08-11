terraform {
  # `terraform test` (the .tftest.hcl runner) requires Terraform >= 1.6 / OpenTofu >= 1.6.
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}
