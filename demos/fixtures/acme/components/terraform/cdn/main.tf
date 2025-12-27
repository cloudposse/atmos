# CDN Component
# Stub component for demo purposes

variable "enabled" {
  type        = bool
  default     = true
  description = "Enable CDN"
}

variable "name" {
  type        = string
  description = "CDN distribution name"
}

variable "origin_domain" {
  type        = string
  default     = ""
  description = "Origin domain"
}

variable "price_class" {
  type        = string
  default     = "PriceClass_100"
  description = "CloudFront price class"
}

output "domain_name" {
  value       = "d1234567890abc.cloudfront.net"
  description = "CloudFront domain name"
}

output "distribution_id" {
  value       = "E1234567890ABC"
  description = "CloudFront distribution ID"
}
