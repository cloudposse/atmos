locals {
  id = join("-", compact([var.namespace, var.tenant, var.environment, var.stage, var.name]))
}

resource "aws_kms_key" "default" {
  description             = "Encryption key for ${local.id}"
  deletion_window_in_days = var.deletion_window_in_days
  enable_key_rotation     = var.enable_key_rotation

  tags = merge(var.tags, { Name = local.id })
}

resource "aws_kms_alias" "default" {
  name          = "alias/${local.id}"
  target_key_id = aws_kms_key.default.key_id
}
