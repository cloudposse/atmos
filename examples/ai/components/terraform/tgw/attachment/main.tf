# Transit Gateway Attachment Component
# Mock Terraform component for demonstrating Atmos AI features.
# This does not create any real cloud resources.
#
# For production use, see the Cloud Posse Transit Gateway component:
# https://github.com/cloudposse-terraform-components/aws-tgw-attachment

terraform {
  required_version = ">= 1.0.0"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
  }
}

resource "null_resource" "tgw_attachment" {
  triggers = {
    appliance_mode_support = var.appliance_mode_support
    dns_support            = var.dns_support
    environment            = lookup(var.tags, "Environment", "unknown")
  }
}
