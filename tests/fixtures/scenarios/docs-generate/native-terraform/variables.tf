variable "bucket_name" {
  description = <<-EOT
    Name of the assets bucket.
    Must be unique across AWS.
  EOT
  type = string
}

variable "configuration" {
  description = "Settings for the application."
  type = object({
    enabled = bool
    ports   = list(number)
  })
  default = {
    enabled = true
    ports   = [80, 443]
  }
}

variable "legacy_mode" {
  description = "OBSOLETE: Use configuration instead."
  type        = bool
  default     = false
}

variable "optional_note" {
  description = "An optional note. Null disables the note."
  type        = string
  default     = null
}

variable "replicas" {
  description = "Number of replicas."
  type        = number
  default     = 2
}
