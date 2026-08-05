output "bucket_name" {
  description = "Name of the bucket created by the native CI E2E fixture."
  value       = aws_s3_bucket.this.bucket
}
