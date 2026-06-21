# Native CI scanner fixture: Checkov target.
#
# A mostly-hardened S3 bucket whose only intentional gap is versioning, so
# Checkov reports a single, clearly attributable finding (CKV_AWS_21). The
# Checkov hook is pinned to that one check because Checkov's default S3 policy
# set also flags architectural choices (cross-region replication, lifecycle,
# event notifications) that are out of scope for this minimal fixture.
#
# Expected scanner: checkov
# Expected rule: CKV_AWS_21 (S3 bucket versioning disabled)

resource "aws_kms_key" "checkov_target" {
  description             = "KMS key for the native CI Checkov target bucket."
  enable_key_rotation     = true
  deletion_window_in_days = 7
}

resource "aws_s3_bucket" "checkov_target" {
  bucket = "atmos-native-ci-e2e-checkov-${var.stage}"
}

resource "aws_s3_bucket_public_access_block" "checkov_target" {
  bucket                  = aws_s3_bucket.checkov_target.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "checkov_target" {
  bucket = aws_s3_bucket.checkov_target.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.checkov_target.arn
    }
  }
}

resource "aws_s3_bucket_logging" "checkov_target" {
  bucket        = aws_s3_bucket.checkov_target.id
  target_bucket = aws_s3_bucket.checkov_target.id
  target_prefix = "log/"
}

# Versioning is intentionally omitted so Checkov reports CKV_AWS_21.
