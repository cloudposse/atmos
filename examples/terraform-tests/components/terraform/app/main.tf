# A small "app backend" that provisions a few real AWS resources. There is no
# providers.tf: the aws/emulator identity and the provider-config contributor
# inject the AWS provider (dummy credentials, path-style S3, skip-flags, and the
# emulator endpoint) automatically, so the same code runs against the local
# emulator and against real AWS unchanged.

resource "aws_s3_bucket" "this" {
  bucket = "${var.name}-${var.environment}"

  tags = {
    Environment = var.environment
    ManagedBy   = "atmos"
  }
}

resource "aws_s3_bucket_versioning" "this" {
  bucket = aws_s3_bucket.this.id

  versioning_configuration {
    status = var.enable_versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_dynamodb_table" "this" {
  name         = "${var.name}-${var.environment}"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }

  tags = {
    Environment = var.environment
  }
}
