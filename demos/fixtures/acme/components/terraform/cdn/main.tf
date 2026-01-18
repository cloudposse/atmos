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

# Variables passed from stack configuration (catalog).

variable "aliases" {
  type        = list(string)
  description = "Alternate domain names for the distribution."
  default     = []
}

variable "default_ttl" {
  type        = number
  description = "Default TTL for cached objects."
  default     = 86400
}

variable "max_ttl" {
  type        = number
  description = "Maximum TTL for cached objects."
  default     = 31536000
}

variable "min_ttl" {
  type        = number
  description = "Minimum TTL for cached objects."
  default     = 0
}

variable "compress" {
  type        = bool
  description = "Enable compression."
  default     = true
}

variable "viewer_protocol_policy" {
  type        = string
  description = "Viewer protocol policy."
  default     = "redirect-to-https"
}

variable "ssl_support_method" {
  type        = string
  description = "SSL support method."
  default     = "sni-only"
}

variable "minimum_protocol_version" {
  type        = string
  description = "Minimum SSL/TLS protocol version."
  default     = "TLSv1.2_2021"
}

variable "logging_enabled" {
  type        = bool
  description = "Enable access logging."
  default     = true
}

variable "waf_enabled" {
  type        = bool
  description = "Enable WAF."
  default     = true
}

variable "geo_restriction_type" {
  type        = string
  description = "Geo restriction type."
  default     = "none"
}

output "domain_name" {
  value       = "d1234567890abc.cloudfront.net"
  description = "CloudFront domain name"
}

output "distribution_id" {
  value       = "E1234567890ABC"
  description = "CloudFront distribution ID"
}
