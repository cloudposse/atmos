resource "aws_s3_bucket" "bucket" {
  bucket = "demo-${var.stage}"
}
