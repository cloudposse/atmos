output "trail_arn" {
  description = "ARN of the CloudTrail trail."
  value       = aws_cloudtrail.this.arn
}

output "log_bucket_id" {
  description = "Name of the audit log bucket."
  value       = aws_s3_bucket.audit_logs.id
}
