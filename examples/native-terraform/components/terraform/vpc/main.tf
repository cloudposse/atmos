# Mock VPC component for demonstrating Atmos migration from native Terraform.
# Uses null_resource so no cloud credentials are needed.

resource "null_resource" "vpc" {
  triggers = {
    cidr_block           = var.cidr_block
    environment          = var.environment
    enable_dns_hostnames = var.enable_dns_hostnames
    tags                 = jsonencode(var.tags)
  }
}
