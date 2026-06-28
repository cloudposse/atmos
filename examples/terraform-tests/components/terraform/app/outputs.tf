output "bucket_id" {
  description = "The name (ID) of the S3 bucket."
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "The ARN of the S3 bucket."
  value       = aws_s3_bucket.this.arn
}

output "table_name" {
  description = "The name of the DynamoDB table."
  value       = aws_dynamodb_table.this.name
}

output "versioning_status" {
  description = "The S3 bucket versioning status (Enabled or Suspended)."
  value       = aws_s3_bucket_versioning.this.versioning_configuration[0].status
}
