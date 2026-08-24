locals {
  name = "${var.project}-${var.stage}"
}

# Central log group for the environment. Applications and services log here
# unless they have a dedicated group.
resource "aws_cloudwatch_log_group" "this" {
  name              = "/${var.project}/${var.stage}"
  retention_in_days = var.retention_in_days
  kms_key_id        = var.kms_key_arn != "" ? var.kms_key_arn : null
}

# Alerts fan out through a single topic per environment; subscribe email,
# chat, or paging integrations to it.
resource "aws_sns_topic" "alerts" {
  name = "${local.name}-alerts"
}

# Alarm on unusual log volume — a cheap, universal signal that something in
# the environment is misbehaving (crash loops, retry storms, debug left on).
resource "aws_cloudwatch_metric_alarm" "log_volume" {
  alarm_name          = "${local.name}-log-volume"
  alarm_description   = "Log volume in ${var.stage} exceeded ${var.alarm_threshold_bytes} bytes over 5 minutes."
  namespace           = "AWS/Logs"
  metric_name         = "IncomingBytes"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.alarm_threshold_bytes
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alerts.arn]

  dimensions = {
    LogGroupName = aws_cloudwatch_log_group.this.name
  }
}
