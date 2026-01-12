# Minimal terraform component for flag inheritance tests
variable "message" {
  type    = string
  default = "hello"
}

output "message" {
  value = var.message
}
