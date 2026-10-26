resource "aws_sqs_queue" "this" {
  name = "atmos-demo-${var.stage}-queue"

  tags = {
    Stage   = var.stage
    Managed = "atmos"
  }
}
