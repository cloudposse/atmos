output "log_group_name" {
  description = "Name of the environment log group."
  value       = aws_cloudwatch_log_group.this.name
}

output "alerts_topic_arn" {
  description = "ARN of the environment alerts SNS topic."
  value       = aws_sns_topic.alerts.arn
}
