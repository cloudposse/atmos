resource "aws_dynamodb_table" "this" {
  name         = "atmos-demo-${var.stage}-table"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }

  tags = {
    Stage   = var.stage
    Managed = "atmos"
  }
}
