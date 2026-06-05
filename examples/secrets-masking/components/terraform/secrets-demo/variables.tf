# Input variables for the secrets masking demo component.

variable "demo_api_key" {
  description = "Demo API key that matches the custom pattern 'demo-key-[A-Za-z0-9]{16}'."
  type        = string
  default     = ""
}

variable "internal_id" {
  description = "Internal ID that matches the custom pattern 'internal-[a-f0-9]{32}'."
  type        = string
  default     = ""
}

variable "custom_token" {
  description = "Custom token that matches the pattern 'tkn_(live|test)_[a-zA-Z0-9]{24}'."
  type        = string
  default     = ""
}

variable "literal_secret" {
  description = "Literal secret value that matches an entry in the literals list."
  type        = string
  default     = ""
}

variable "plain_value" {
  description = "Plain value that should NOT be masked."
  type        = string
  default     = ""
}
