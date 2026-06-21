# Native CI scanner fixture: KICS target.
#
# A deliberately minimal S3 bucket: the bucket itself is the only resource so it
# applies cleanly against the Floci emulator in the terraform-apply E2E (Floci
# cannot create S3 bucket logging). Scanner noise is controlled in the hook
# instead — the KICS hook includes only the versioning query, so this target
# produces exactly one clearly-attributable finding.
#
# Expected scanner: kics
# Expected finding: S3 Bucket Without Versioning (568a4d22-3517-44a6-a7ad-6a7eed88722c)

resource "aws_s3_bucket" "kics_target" {
  bucket = "atmos-native-ci-e2e-kics-${var.stage}"
}
