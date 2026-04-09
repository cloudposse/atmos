variable "enabled" {
  type    = bool
  default = true
}

variable "cidr" {
  type    = string
  default = "10.0.0.0/16"
}

output "enabled" {
  value = var.enabled
}

output "cidr" {
  value = var.cidr
}
