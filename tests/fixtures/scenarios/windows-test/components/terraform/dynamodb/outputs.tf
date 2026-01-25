output "table_name" {
  value       = module.dynamodb_table.table_name
  description = "DynamoDB table name"
}

output "table_id" {
  value       = module.dynamodb_table.table_id
  description = "DynamoDB table ID"
}

output "table_arn" {
  value       = module.dynamodb_table.table_arn
  description = "DynamoDB table ARN"
}

output "global_secondary_index_names" {
  value       = module.dynamodb_table.global_secondary_index_names
  description = "DynamoDB global secondary index names"
}

output "local_secondary_index_names" {
  value       = module.dynamodb_table.local_secondary_index_names
  description = "DynamoDB local secondary index names"
}

output "table_stream_arn" {
  value       = module.dynamodb_table.table_stream_arn
  description = "DynamoDB table stream ARN"
}

output "table_stream_label" {
  value       = module.dynamodb_table.table_stream_label
  description = "DynamoDB table stream label"
}

output "hash_key" {
  value       = var.hash_key
  description = "DynamoDB table hash key"
}

output "range_key" {
  value       = var.range_key
  description = "DynamoDB table range key"
}
