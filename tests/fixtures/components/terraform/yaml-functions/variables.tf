variable "stage" {
  description = "Stage where it will be deployed"
  type        = string
}

variable "string_var" {
  description = "String variable"
  type        = string
  default     = "test"
}

variable "boolean_var" {
  description = "Boolean variable"
  type        = bool
  default     = false
}

variable "list_var" {
  description = "List variable"
  type        = list(string)
  default     = []
}

variable "map_var" {
  description = "Map variable"
  type        = map(string)
  default     = {}
}
