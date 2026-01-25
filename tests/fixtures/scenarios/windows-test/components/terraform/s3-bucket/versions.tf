terraform {
  required_version = ">= 1.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 4.0, < 6.0.0"
    }
    template = {
      source  = "cloudposse/template"
      version = ">= 2.2.0"
    }
  }
}
