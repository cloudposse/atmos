output "retention_seconds" {
  value = aws_sqs_queue.app.message_retention_seconds
}
