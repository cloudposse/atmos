terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

# Bucket containing the application assets.
resource "aws_s3_bucket" "assets" {
  bucket = var.bucket_name
}

data "aws_caller_identity" "current" {}

# Shared naming convention.
module "label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"
}

# Remote state submodule.
module "state" {
  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.5.0"
}

# Local helper module.
module "helper" {
  source = "../helper"
}

output "bucket_id" {
  description = <<-EOT
    Identifier of the assets bucket.
    Use this when configuring access policies.
  EOT
  value = aws_s3_bucket.assets.id
}

output "legacy_id" {
  description = "OBSOLETE: Use bucket_id instead."
  value       = aws_s3_bucket.assets.id
}
