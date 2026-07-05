locals {
  name = "${var.project}-${var.stage}"
}

resource "aws_s3_bucket" "assets" {
  bucket        = "${local.name}-assets"
  force_destroy = var.force_destroy
}

resource "aws_sqs_queue" "work" {
  name                       = "${local.name}-work"
  visibility_timeout_seconds = var.queue_visibility_timeout_seconds
}

resource "aws_ssm_parameter" "metadata" {
  for_each = var.parameters

  name  = "/${var.project}/${var.stage}/app/${each.key}"
  type  = "String"
  value = each.value
}
