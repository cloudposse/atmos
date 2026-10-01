resource "aws_ssm_parameter" "marker" {
  name  = "/atmos/demo/${var.stage}/marker"
  type  = "String"
  value = "hello from ${var.stage}"
}
