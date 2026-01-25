terraform {
  required_version = ">= 1.3.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 4.0, < 6.0.0"
    }
    sops = {
      source  = "carlpett/sops"
      version = ">= 0.5, < 1.0"
    }
  }
}
