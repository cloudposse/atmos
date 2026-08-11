terraform {
  required_version = ">= 1.3.0"
}

variable "api_key" {
  type      = string
  sensitive = true
}
