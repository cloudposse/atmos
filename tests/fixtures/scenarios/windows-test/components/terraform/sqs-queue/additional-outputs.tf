# Additional outputs for our implementation
# These outputs are not included in the upstream component.
# We put them in a separate file so they don't get overwritten when vendoring.

output "region" {
  description = "AWS region of the SQS queue"
  value       = var.region
}
