# Transit Gateway Hub Component
# Mock Terraform component for demonstrating Atmos AI features.
# This does not create any real cloud resources.
#
# For production use, see the Cloud Posse Transit Gateway components:
# https://github.com/cloudposse-terraform-components/aws-tgw-hub
# https://github.com/cloudposse-terraform-components/aws-tgw-routes

terraform {
  required_version = ">= 1.0.0"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
  }
}

resource "null_resource" "tgw_hub" {
  triggers = {
    amazon_side_asn          = var.amazon_side_asn
    auto_accept_shared       = var.auto_accept_shared_attachments
    default_route_table_assn = var.default_route_table_association
    environment              = lookup(var.tags, "Environment", "unknown")
  }
}
