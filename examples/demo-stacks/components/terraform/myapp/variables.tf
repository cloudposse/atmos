variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "location" {
  description = "Location for which the weather. Supports city name, 3-letter airport code, area code, or GPS coordinates."
  type        = string
  default     = "Los Angeles"
}

variable "options" {
  description = "Options to customize the output. '0' for no colors, 'T' for terminal output."
  type        = string
  default     = "0T"
}

variable "format" {
  description = "Format of the output. 'v2' for the new version of the output format."
  type        = string
  default     = "v2"
}

variable "lang" {
  description = "Language in which the weather is displayed. 'en' for English."
  type        = string
  default     = "en"
}

variable "units" {
  description = "Units in which the weather is displayed. 'm' for metric units."
  type        = string
  default     = "m"
}
