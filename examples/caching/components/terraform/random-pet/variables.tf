variable "stage" {
  description = "Deployment stage (provided by the stack via name_pattern)."
  type        = string
}

variable "length" {
  description = "Number of words in the generated pet name."
  type        = number
  default     = 2
}

variable "separator" {
  description = "Separator between words in the generated pet name."
  type        = string
  default     = "-"
}
