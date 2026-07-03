# Environment metadata published to SSM Parameter Store so other components,
# scripts, and applications can discover it at a well-known path.
resource "aws_ssm_parameter" "metadata" {
  for_each = var.parameters

  name  = "/${var.project}/${var.stage}/${each.key}"
  type  = "String"
  value = each.value
}
