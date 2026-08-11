locals {
  id = join("-", compact([var.namespace, var.tenant, var.environment, var.stage, var.name]))
}

resource "aws_sns_topic" "default" {
  name              = local.id
  kms_master_key_id = var.kms_key_arn != "" ? var.kms_key_arn : null

  tags = merge(var.tags, { Name = local.id })
}
