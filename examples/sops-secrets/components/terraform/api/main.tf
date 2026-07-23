variable "datadog_api_key" {
  type      = string
  default   = ""
  sensitive = true
}

variable "redis_url" {
  type    = string
  default = ""
}

variable "stage" {
  type    = string
  default = ""
}

output "stage" {
  value = var.stage
}

output "redis_url" {
  value = var.redis_url
}
