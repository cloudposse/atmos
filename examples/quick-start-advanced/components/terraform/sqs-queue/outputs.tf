output "queue_url" {
  description = "The URL (ID) of the SQS queue."
  value       = aws_sqs_queue.default.id
}

output "queue_arn" {
  description = "The ARN of the SQS queue."
  value       = aws_sqs_queue.default.arn
}
