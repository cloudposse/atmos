# VPC Component
# Mock Terraform component for demonstrating Atmos AI features.
# This does not create any real cloud resources.
#
# For production use, see the Cloud Posse VPC component:
# https://github.com/cloudposse-terraform-components/aws-vpc

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
    vpc_cidr            = var.vpc_cidr
    availability_zones  = join(",", var.availability_zones)
    nat_gateway_enabled = var.nat_gateway_enabled
    environment         = lookup(var.tags, "Environment", "unknown")
  }
}

locals {
  subnet_count = length(var.availability_zones)
}
