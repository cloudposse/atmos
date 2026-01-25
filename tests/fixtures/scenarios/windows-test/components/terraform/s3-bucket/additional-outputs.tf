# Additional outputs for our implementation
# These outputs are not included in the upstream component.
# We put them in a separate file so they don't get overwritten when vendoring.

output "bucket_arn_objects" {
  value       = "${module.s3_bucket.bucket_arn}/*"
  description = "Bucket ARN with wildcard for all objects"
}
