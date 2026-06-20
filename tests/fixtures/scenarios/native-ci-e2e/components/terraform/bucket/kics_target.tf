# Native CI scanner fixture: KICS target.
# Keep this finding isolated so visual PR annotations identify KICS.
#
# Expected scanner: kics
# Expected category: S3 bucket security misconfiguration
#
#
#
#
#
#
#
#
#

resource "aws_s3_bucket" "kics_target" {
  bucket = "atmos-native-ci-e2e-kics-${var.stage}"
}
