variable "name" {
  type    = string
  default = "vpc"
}

output "name" {
  value = var.name
}
