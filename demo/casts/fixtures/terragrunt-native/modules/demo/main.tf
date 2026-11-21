variable "greeting" {
  type = string
}

resource "terraform_data" "demo" {
  input = var.greeting
}

output "greeting" {
  value = terraform_data.demo.output
}
