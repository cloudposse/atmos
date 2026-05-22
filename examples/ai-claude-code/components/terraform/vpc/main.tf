# VPC Component
# Mock Terraform component for demonstrating Atmos AI features.
# This does not create any real cloud resources.

terraform {
  required_version = ">= 1.0.0"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
  }
}

resource "null_resource" "vpc" {
  triggers = {
    vpc_cidr           = var.vpc_cidr
    availability_zones = join(",", var.availability_zones)
    environment        = lookup(var.tags, "Environment", "unknown")
  }
}
