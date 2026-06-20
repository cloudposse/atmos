# Native CI scanner fixture: Checkov target.
# Keep this finding isolated so visual PR annotations identify Checkov.
#
# Expected scanner: checkov
# Expected rule: CKV_AWS_21

resource "aws_s3_bucket" "checkov_target" {
  bucket = "atmos-native-ci-e2e-checkov-${var.stage}"
}
