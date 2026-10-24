# Deliberately minimal S3 bucket: no encryption, no versioning, no
# logging, no public-access block. Scanners flag a handful of findings
# (3-5 typically) — enough to demonstrate the hook end-to-end without
# overwhelming the demo output.

resource "aws_s3_bucket" "data" {
  bucket = "example-${var.environment}-data"

  tags = {
    Environment = var.environment
  }
}
