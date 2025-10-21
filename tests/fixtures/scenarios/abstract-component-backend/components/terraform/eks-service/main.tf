variable "name" {
  type = string
}

variable "namespace" {
  type = string
}

variable "stage" {
  type = string
}

output "name" {
  value = var.name
}
