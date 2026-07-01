# Minimal S3 bucket. The `unused` variable below is declared but never
# referenced, so tflint's builtin `terraform_unused_declarations` rule
# (on by default, no plugins required) flags it — enough to demonstrate
# the hook end-to-end without any .tflint.hcl configuration.

resource "aws_s3_bucket" "data" {
  bucket = "example-${var.environment}-data"

  tags = {
    Environment = var.environment
  }
}
