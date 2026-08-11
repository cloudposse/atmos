resource "aws_s3_bucket" "this" {
  bucket = "atmos-demo-${var.stage}-bucket"

  tags = {
    Stage   = var.stage
    Managed = "atmos"
  }
}
