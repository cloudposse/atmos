locals {
  id = join("-", compact([var.namespace, var.tenant, var.environment, var.stage, var.name]))
}

resource "aws_dynamodb_table" "default" {
  name         = local.id
  billing_mode = var.billing_mode
  hash_key     = var.hash_key

  attribute {
    name = var.hash_key
    type = "S"
  }

  tags = merge(var.tags, { Name = local.id })
}
