variable "name" {
  type        = string
  description = "Name of the application."
}

output "name" {
  value = var.name
}
