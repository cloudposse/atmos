resource "aws_s3_bucket" "this" {
  bucket = "atmos-native-ci-e2e-${var.stage}"

  tags = {
    AtmosFixture = "native-ci-e2e"
    Stage        = var.stage
  }
}
