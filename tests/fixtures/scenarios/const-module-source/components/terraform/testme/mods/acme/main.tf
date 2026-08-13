variable "name" {
  type    = string
  default = "acme"
}

output "greeting" {
  value = "hello ${var.name}"
}
