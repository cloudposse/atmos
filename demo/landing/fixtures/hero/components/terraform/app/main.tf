# Use the AWS provider against the local emulator, including its propagation waits.
resource "aws_sqs_queue" "app" {
  name                      = "atmos-hero-${var.stage}"
  message_retention_seconds = var.retention_seconds
}
