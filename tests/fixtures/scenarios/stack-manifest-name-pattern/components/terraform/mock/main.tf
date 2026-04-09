variable "enabled" {
  type    = bool
  default = true
}

variable "cidr" {
  type    = string
  default = "10.0.0.0/16"
}

output "vpc_id" {
  value = "vpc-mock-12345"
}
