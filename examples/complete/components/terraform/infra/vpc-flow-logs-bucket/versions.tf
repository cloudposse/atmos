terraform {
  required_version = ">= 0.13.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 3.0"
    }
    template = {
      source  = "cloudposse/template"
      version = ">= 2.2"
    }
    local = {
      source  = "hashicorp/local"
      version = ">= 1.3"
    }
  }
}
