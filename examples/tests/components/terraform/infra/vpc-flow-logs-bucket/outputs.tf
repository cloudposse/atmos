output "vpc_flow_logs_bucket_id" {
  value       = module.flow_logs_s3_bucket.bucket_id
  description = "VPC Flow Logs bucket ID"
}

output "vpc_flow_logs_bucket_arn" {
  value       = module.flow_logs_s3_bucket.bucket_arn
  description = "VPC Flow Logs bucket ARN"
}
