variable "example" {
  type        = string
  description = "testing variable"
}

variable "enable_failure" {
  type        = bool
  default     = false
  description = "Always fail"
}

variable "enable_warning" {
  type        = bool
  default     = false
  description = "Enable warning"
}
