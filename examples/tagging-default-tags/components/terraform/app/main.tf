# The AWS provider supplies the tags; the resource needs no tags argument.
resource "aws_s3_bucket" "this" {
  bucket_prefix = "atmos-tagging-example-"
}
