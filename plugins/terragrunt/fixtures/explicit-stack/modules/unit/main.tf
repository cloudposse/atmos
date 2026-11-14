variable "payload" {
  type = any
}

resource "terraform_data" "unit" {
  input = var.payload
}

output "value" {
  value = terraform_data.unit.output
}
