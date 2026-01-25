# These event definitions, queue policies, and SQS queue definition
# come from the Karpenter CloudFormation template.
# See comments in `controller-policy.tf` for more information.

locals {
  interruption_handler_enabled    = local.enabled && var.interruption_handler_enabled
  interruption_handler_queue_name = module.this.id
  interruption_handler_queue_arn  = one(aws_sqs_queue.interruption_handler[*].arn)

  dns_suffix = join("", data.aws_partition.current[*].dns_suffix)

  events = {
    health_event = {
      name        = "HealthEvent"
      description = "Karpenter interrupt - AWS health event"
      event_pattern = {
        source      = ["aws.health"]
        detail-type = ["AWS Health Event"]
      }
    }
    spot_interupt = {
      name        = "SpotInterrupt"
      description = "Karpenter interrupt - EC2 spot instance interruption warning"
      event_pattern = {
        source      = ["aws.ec2"]
        detail-type = ["EC2 Spot Instance Interruption Warning"]
      }
    }
    instance_rebalance = {
      name        = "InstanceRebalance"
      description = "Karpenter interrupt - EC2 instance rebalance recommendation"
      event_pattern = {
        source      = ["aws.ec2"]
        detail-type = ["EC2 Instance Rebalance Recommendation"]
      }
    }
    instance_state_change = {
      name        = "InstanceStateChange"
      description = "Karpenter interrupt - EC2 instance state-change notification"
      event_pattern = {
        source      = ["aws.ec2"]
        detail-type = ["EC2 Instance State-change Notification"]
      }
    }
  }
}

resource "aws_sqs_queue" "interruption_handler" {
  count = local.interruption_handler_enabled ? 1 : 0

  name                      = local.interruption_handler_queue_name
  message_retention_seconds = var.interruption_queue_message_retention
  sqs_managed_sse_enabled   = true

  tags = module.this.tags
}

data "aws_iam_policy_document" "interruption_handler" {
  count = local.interruption_handler_enabled ? 1 : 0

  statement {
    sid       = "SqsWrite"
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.interruption_handler[0].arn]

    principals {
      type = "Service"
      identifiers = [
        "events.${local.dns_suffix}",
        "sqs.${local.dns_suffix}",
      ]
    }
  }
}

resource "aws_sqs_queue_policy" "interruption_handler" {
  count = local.interruption_handler_enabled ? 1 : 0

  queue_url = aws_sqs_queue.interruption_handler[0].url
  policy    = data.aws_iam_policy_document.interruption_handler[0].json
}

resource "aws_cloudwatch_event_rule" "interruption_handler" {
  for_each = { for k, v in local.events : k => v if local.interruption_handler_enabled }

  name          = "${module.this.id}-${each.value.name}"
  description   = each.value.description
  event_pattern = jsonencode(each.value.event_pattern)

  tags = module.this.tags
}

resource "aws_cloudwatch_event_target" "interruption_handler" {
  for_each = { for k, v in local.events : k => v if local.interruption_handler_enabled }

  rule      = aws_cloudwatch_event_rule.interruption_handler[each.key].name
  target_id = "KarpenterInterruptionQueueTarget"
  arn       = aws_sqs_queue.interruption_handler[0].arn
}
