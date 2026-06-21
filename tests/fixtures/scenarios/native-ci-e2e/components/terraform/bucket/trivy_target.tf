# Native CI scanner fixture: Trivy target.
#
# A deliberately minimal S3 bucket: the bucket itself is the only resource so it
# applies cleanly against the Floci emulator in the terraform-apply E2E (Floci
# cannot create KMS keys or S3 bucket logging). Scanner noise is controlled in
# the hook instead — the Trivy hook filters to MEDIUM severity, and on a bare
# bucket AWS-0090 (versioning) is the only MEDIUM check, so this target produces
# exactly one clearly-attributable finding.
#
# Expected scanner: trivy
# Expected finding: AWS-0090 (S3 bucket does not have versioning enabled)

resource "aws_s3_bucket" "trivy_target" {
  bucket = "atmos-native-ci-e2e-trivy-${var.stage}"
}
