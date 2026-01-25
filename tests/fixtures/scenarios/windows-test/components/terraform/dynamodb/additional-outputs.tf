# Additional outputs for our implementation
# These outputs are not included in the upstream component.
# We put them in a separate file so they don't get overwritten when vendoring.

output "region" {
  value       = var.region
  description = "AWS region of the DynamoDB table"
}

output "table_arn_indexes" {
  value       = "${module.dynamodb_table.table_arn}/index/*"
  description = "Table ARN with wildcard for all indexes"
}
