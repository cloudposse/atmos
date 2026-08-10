locals {
  id = join("-", compact([var.namespace, var.tenant, var.environment, var.stage, var.name]))
}

resource "aws_s3_bucket" "default" {
  bucket        = local.id
  force_destroy = var.force_destroy

  tags = merge(var.tags, { Name = local.id })
}

resource "aws_s3_bucket_versioning" "default" {
  bucket = aws_s3_bucket.default.id

  versioning_configuration {
    status = var.versioning_enabled ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "default" {
  bucket = aws_s3_bucket.default.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = var.kms_key_arn
    }
  }
}
