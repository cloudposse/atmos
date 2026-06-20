# Native CI scanner fixture: Trivy target.
# Keep this finding isolated so visual PR annotations identify Trivy.
#
# Expected scanner: trivy
# Expected category: S3 bucket security misconfiguration
#
#
#
#
#

resource "aws_s3_bucket" "trivy_target" {
  bucket = "atmos-native-ci-e2e-trivy-${var.stage}"
}
