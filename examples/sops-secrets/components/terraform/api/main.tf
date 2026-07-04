variable "datadog_api_key" {
  type      = string
  default   = ""
  sensitive = true
}

variable "redis_url" {
  type    = string
  default = ""
}

output "redis_url" {
  value = var.redis_url
}
