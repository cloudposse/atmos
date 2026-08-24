output "stage" {
  description = "Stage of deployment."
  value       = var.stage
}

output "queue_url" {
  description = "URL of the SQS queue created in the local AWS emulator."
  value       = aws_sqs_queue.this.url
}
