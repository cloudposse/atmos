locals {
  name = "${var.project}-${var.stage}"
}

resource "aws_s3_bucket" "assets" {
  bucket        = "${local.name}-assets"
  force_destroy = var.force_destroy
}

resource "aws_s3_bucket_public_access_block" "assets" {
  bucket = aws_s3_bucket.assets.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "assets" {
  bucket = aws_s3_bucket.assets.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_sqs_queue" "work" {
  name                       = "${local.name}-work"
  visibility_timeout_seconds = var.queue_visibility_timeout_seconds
}

resource "aws_ssm_parameter" "metadata" {
  for_each = var.parameters

  name  = "/${var.project}/${var.stage}/app/${each.key}"
  type  = "String"
  value = each.value
}
