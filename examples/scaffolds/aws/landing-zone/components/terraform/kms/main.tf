resource "aws_kms_key" "this" {
  description             = "${var.project}-${var.stage} baseline encryption key"
  deletion_window_in_days = var.deletion_window_in_days
  enable_key_rotation     = true
}

resource "aws_kms_alias" "this" {
  name          = "alias/${var.project}-${var.stage}"
  target_key_id = aws_kms_key.this.key_id
}
