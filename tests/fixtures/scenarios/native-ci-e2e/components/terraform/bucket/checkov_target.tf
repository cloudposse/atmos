# Native CI scanner fixture: Checkov target.
#
# A deliberately minimal S3 bucket: the bucket itself is the only resource so it
# applies cleanly against the Floci emulator in the terraform-apply E2E (Floci
# cannot create KMS keys or S3 bucket logging). Scanner noise is controlled in
# the hook instead — the Checkov hook is pinned to a single check (CKV_AWS_21)
# so this target produces exactly one clearly-attributable finding.
#
# Expected scanner: checkov
# Expected rule: CKV_AWS_21 (S3 bucket versioning disabled)

resource "aws_s3_bucket" "checkov_target" {
  bucket = "atmos-native-ci-e2e-checkov-${var.stage}"
}
