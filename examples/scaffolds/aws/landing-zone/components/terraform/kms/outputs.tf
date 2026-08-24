output "key_arn" {
  description = "ARN of the baseline KMS key."
  value       = aws_kms_key.this.arn
}

output "alias_name" {
  description = "Alias of the baseline KMS key."
  value       = aws_kms_alias.this.name
}
