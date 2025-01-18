variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "location" {
  description = "Location for which the weather."
  type        = string
  default     = "Los Angeles"
}

variable "options" {
  description = "Options to customize the output."
  type        = string
  default     = "0T"
}

variable "format" {
  description = "Format of the output."
  type        = string
  default     = "v2"
}

variable "lang" {
  description = "Language in which the weather is displayed."
  type        = string
  default     = "en"
}

variable "units" {
  description = "Units in which the weather is displayed."
  type        = string
  default     = "m"
}
