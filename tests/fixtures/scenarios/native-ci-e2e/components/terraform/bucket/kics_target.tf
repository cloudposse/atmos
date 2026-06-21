# Native CI scanner fixture: KICS target.
#
# A mostly-hardened S3 bucket whose only intentional gap is versioning, so
# KICS reports a single, clearly attributable finding. KICS does not check S3
# encryption or public-access-block, and adding a KMS key would introduce
# unrelated KICS findings, so this target is intentionally leaner than the
# Checkov/Trivy targets. The account-level "IAM Access Analyzer Not Enabled"
# query is excluded in the hook because it is not a per-bucket property.
#
# Expected scanner: kics
# Expected finding: S3 Bucket Without Versioning (568a4d22-3517-44a6-a7ad-6a7eed88722c)

resource "aws_s3_bucket" "kics_target" {
  bucket = "atmos-native-ci-e2e-kics-${var.stage}"

  tags = {
    AtmosFixture = "native-ci-e2e"
    Stage        = var.stage
  }
}

resource "aws_s3_bucket_logging" "kics_target" {
  bucket        = aws_s3_bucket.kics_target.id
  target_bucket = aws_s3_bucket.kics_target.id
  target_prefix = "log/"
}

# Versioning is intentionally omitted so KICS reports "S3 Bucket Without Versioning".
