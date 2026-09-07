# Environment metadata published to SSM Parameter Store so other components,
# scripts, and applications can discover it at a well-known path.
resource "aws_ssm_parameter" "metadata" {
  for_each = var.parameters

  name   = "/${var.project}/${var.stage}/${each.key}"
  type   = var.kms_key_arn == "" ? "String" : "SecureString"
  value  = each.value
  key_id = var.kms_key_arn == "" ? null : var.kms_key_arn
}
