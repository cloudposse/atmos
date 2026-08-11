locals {
  id     = join("-", compact([var.namespace, var.tenant, var.environment, var.stage, var.name]))
  prefix = "/${var.namespace}/${var.tenant}/${var.stage}/${var.name}"
  config = {
    bucket_id  = var.bucket_id
    table_name = var.table_name
    topic_arn  = var.topic_arn
    queue_url  = var.queue_url
  }
}

resource "aws_ssm_parameter" "config" {
  for_each = local.config
  name     = "${local.prefix}/${each.key}"
  type     = "String"
  value    = each.value
  tags     = var.tags
}

resource "aws_ssm_parameter" "api_key" {
  name   = "${local.prefix}/api_key"
  type   = "SecureString"
  value  = var.api_key
  key_id = var.kms_key_arn != "" ? var.kms_key_arn : null
  tags   = var.tags
}

resource "aws_ssm_parameter" "db_password" {
  name   = "${local.prefix}/db_password"
  type   = "SecureString"
  value  = var.db_password
  key_id = var.kms_key_arn != "" ? var.kms_key_arn : null
  tags   = var.tags
}
