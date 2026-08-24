terraform {
  required_version = ">= 1.3.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

variable "name" {
  type        = string
  description = "Component name used to build the resource identifier."
}

variable "stage" {
  type        = string
  description = "Stage name."
  default     = ""
}

variable "kms_key_arn" {
  type        = string
  description = "KMS key ARN used to encrypt the bucket (from the kms component)."
  default     = ""
}

locals {
  id = "emu-native-${var.stage}-${var.name}"
}

resource "aws_s3_bucket" "this" {
  bucket        = local.id
  force_destroy = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "this" {
  count  = var.kms_key_arn != "" ? 1 : 0
  bucket = aws_s3_bucket.this.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = var.kms_key_arn
    }
  }
}

output "bucket_id" {
  value       = aws_s3_bucket.this.id
  description = "Name of the S3 bucket."
}
