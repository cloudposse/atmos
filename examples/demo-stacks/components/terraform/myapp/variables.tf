variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "location" {
  description = "Location configured for the component."
  type        = string
  default     = "Los Angeles"
}

variable "options" {
  description = "Options to customize the output."
  type        = string
  default     = "0T"
}

variable "format" {
  description = "Format label written to the demo artifact."
  type        = string
  default     = "v2"
}

variable "lang" {
  description = "Language configured for the component."
  type        = string
  default     = "en"
}

variable "units" {
  description = "Units configured for the component."
  type        = string
  default     = "m"
}
