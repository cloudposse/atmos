output "stage" {
  description = "Stage of deployment."
  value       = var.stage
}

output "bucket_name" {
  description = "Name of the S3 bucket created in the local AWS emulator."
  value       = aws_s3_bucket.this.bucket
}
