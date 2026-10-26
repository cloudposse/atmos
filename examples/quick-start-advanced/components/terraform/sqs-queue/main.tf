locals {
  id = join("-", compact([var.namespace, var.tenant, var.environment, var.stage, var.name]))
}

resource "aws_sqs_queue" "default" {
  name                       = local.id
  visibility_timeout_seconds = var.visibility_timeout_seconds
  message_retention_seconds  = var.message_retention_seconds

  tags = merge(var.tags, { Name = local.id })
}

resource "aws_sns_topic_subscription" "default" {
  count = var.topic_arn != "" ? 1 : 0

  topic_arn = var.topic_arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.default.arn

  # Attach the queue policy that lets SNS deliver before AWS validates the
  # subscription, otherwise the first apply can be rejected as a race.
  depends_on = [aws_sqs_queue_policy.default]
}

resource "aws_sqs_queue_policy" "default" {
  count = var.topic_arn != "" ? 1 : 0

  queue_url = aws_sqs_queue.default.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "sns.amazonaws.com"
        }
        Action   = "sqs:SendMessage"
        Resource = aws_sqs_queue.default.arn
        Condition = {
          ArnEquals = {
            "aws:SourceArn" = var.topic_arn
          }
        }
      }
    ]
  })
}
