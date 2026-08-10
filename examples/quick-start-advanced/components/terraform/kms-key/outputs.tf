output "key_id" {
  description = "The globally unique identifier for the KMS key."
  value       = aws_kms_key.default.key_id
}

output "key_arn" {
  description = "The ARN of the KMS key."
  value       = aws_kms_key.default.arn
}

output "alias_name" {
  description = "The display name of the KMS alias."
  value       = aws_kms_alias.default.name
}
