output "stage" {
  description = "Stage of deployment."
  value       = var.stage
}

output "table_name" {
  description = "Name of the DynamoDB table created in the local AWS emulator."
  value       = aws_dynamodb_table.this.name
}
