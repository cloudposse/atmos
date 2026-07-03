# Native CI scanner fixture: KICS target.
#
# A minimal S3 bucket that applies cleanly against the Floci emulator in the
# terraform-apply E2E (Floci cannot create S3 bucket logging). The KICS hook
# includes two queries — versioning and logging — and the logging query is
# disabled inline below via a file-level kics-scan directive. This exercises
# KICS's native ignore handling end-to-end — without it both findings would
# surface. Only the versioning finding should appear.
#
# Expected scanner:   kics
# Expected finding:   S3 Bucket Without Versioning (568a4d22-3517-44a6-a7ad-6a7eed88722c)
# Suppressed inline:  S3 Bucket Logging Disabled (f861041c-8c9f-4156-acfc-5e6e524f5884)

# kics-scan disable=f861041c-8c9f-4156-acfc-5e6e524f5884
resource "aws_s3_bucket" "kics_target" {
  bucket = "atmos-native-ci-e2e-kics-${var.stage}"
}
