# Native CI scanner fixture: Trivy target.
#
# A minimal S3 bucket plus a public-access block — both apply cleanly against
# the Floci emulator in the terraform-apply E2E (Floci cannot create KMS keys or
# S3 bucket logging). The Trivy hook is scoped to LOW+MEDIUM severities, leaving
# versioning (AWS-0090, MEDIUM) and logging (AWS-0089, LOW); the logging finding
# is suppressed inline below. This exercises Trivy's native ignore directive
# end-to-end — without it both findings would surface. Only the versioning
# finding should appear.
#
# Expected scanner:   trivy
# Expected finding:   AWS-0090 (S3 bucket versioning disabled)
# Suppressed inline:  AWS-0089 (S3 bucket logging disabled)

#trivy:ignore:AWS-0089
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
