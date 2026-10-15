variable "name" {
  type = string
}

resource "terraform_data" "nested" {
  input = var.name
}
