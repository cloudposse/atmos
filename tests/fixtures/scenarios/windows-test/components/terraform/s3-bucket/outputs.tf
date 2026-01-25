output "bucket_domain_name" {
  value       = module.s3_bucket.bucket_domain_name
  description = "Bucket domain name"
}

output "bucket_regional_domain_name" {
  value       = module.s3_bucket.bucket_regional_domain_name
  description = "Bucket region-specific domain name"
}

output "bucket_id" {
  value       = module.s3_bucket.bucket_id
  description = "Bucket ID"
}

output "bucket_arn" {
  value       = module.s3_bucket.bucket_arn
  description = "Bucket ARN"
}

output "bucket_region" {
  value       = module.s3_bucket.bucket_region
  description = "Bucket region"
}
