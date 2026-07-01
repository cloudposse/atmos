output "bucket_id" {
  description = "The name (ID) of the created S3 bucket."
  value       = aws_s3_bucket.this.id
}

output "stage" {
  description = "The deployment stage."
  value       = var.stage
}
