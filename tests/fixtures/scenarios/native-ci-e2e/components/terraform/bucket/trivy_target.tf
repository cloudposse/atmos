# Native CI scanner fixture: Trivy target.
#
# A mostly-hardened S3 bucket whose only intentional gap is versioning, so
# Trivy reports a single, clearly attributable finding (AWS-0090). Public
# access block, customer-managed-key encryption, and logging are all
# configured so the rest of Trivy's S3 checks pass.
#
# Expected scanner: trivy
# Expected finding: AWS-0090 (S3 bucket does not have versioning enabled)

resource "aws_kms_key" "trivy_target" {
  description             = "KMS key for the native CI Trivy target bucket."
  enable_key_rotation     = true
  deletion_window_in_days = 7
}

resource "aws_s3_bucket" "trivy_target" {
  bucket = "atmos-native-ci-e2e-trivy-${var.stage}"
}

resource "aws_s3_bucket_public_access_block" "trivy_target" {
  bucket                  = aws_s3_bucket.trivy_target.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "trivy_target" {
  bucket = aws_s3_bucket.trivy_target.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.trivy_target.arn
    }
  }
}

resource "aws_s3_bucket_logging" "trivy_target" {
  bucket        = aws_s3_bucket.trivy_target.id
  target_bucket = aws_s3_bucket.trivy_target.id
  target_prefix = "log/"
}

# Versioning is intentionally omitted so Trivy reports AWS-0090.
