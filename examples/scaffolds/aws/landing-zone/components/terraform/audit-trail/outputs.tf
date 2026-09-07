output "log_bucket_id" {
  description = "Name of the audit log bucket."
  value       = aws_s3_bucket.audit_logs.id
}

output "log_bucket_arn" {
  description = "ARN of the audit log bucket."
  value       = aws_s3_bucket.audit_logs.arn
}
