variable "datadog_api_key" {
  type      = string
  default   = ""
  sensitive = true
}

variable "db_password" {
  type      = string
  default   = ""
  sensitive = true
}

output "has_datadog_api_key" {
  value = var.datadog_api_key != ""
}
