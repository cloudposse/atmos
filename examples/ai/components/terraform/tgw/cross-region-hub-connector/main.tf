# Transit Gateway Cross-Region Hub Connector Component
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

resource "null_resource" "tgw_cross_region_hub_connector" {
  triggers = {
    peer_region = var.peer_region
    environment = lookup(var.tags, "Environment", "unknown")
  }
}
