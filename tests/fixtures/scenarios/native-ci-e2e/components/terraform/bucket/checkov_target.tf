# Native CI scanner fixture: Checkov target.
#
# A minimal S3 bucket that applies cleanly against the Floci emulator in the
# terraform-apply E2E (Floci cannot create KMS keys or S3 bucket logging). The
# Checkov hook is scoped to two checks — versioning (CKV_AWS_21) and access
# logging (CKV_AWS_18) — and the logging check is suppressed inline below. This
# exercises Checkov's native skip directive end-to-end: if Atmos did not run
# Checkov where it can read the directive, both checks would surface. Only the
# versioning finding should appear.
#
# Expected scanner:   checkov
# Expected finding:   CKV_AWS_21 (S3 bucket versioning disabled)
# Suppressed inline:  CKV_AWS_18 (S3 bucket access logging disabled)

resource "aws_s3_bucket" "checkov_target" {
  #checkov:skip=CKV_AWS_18:Suppressed inline to verify Atmos honors Checkov skip directives.
  bucket = "atmos-native-ci-e2e-checkov-${var.stage}"
}
