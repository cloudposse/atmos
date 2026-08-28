output "bucket_name" {
  description = "Name of the application asset bucket."
  value       = aws_s3_bucket.assets.id
}

output "queue_url" {
  description = "URL of the application work queue."
  value       = aws_sqs_queue.work.url
}

output "parameter_names" {
  description = "Names of the published SSM parameters."
  value       = [for p in aws_ssm_parameter.metadata : p.name]
}
