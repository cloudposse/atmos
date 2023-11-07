variable "region" {
  type        = string
  description = "Region"
}

variable "service_1_name" {
  type        = string
  description = "Service 1 name"
}

variable "service_1_list" {
  type        = list(string)
  description = "Service 1 list"
  default     = []
}

variable "service_1_map" {
  type        = map(string)
  description = "Service 1 map"
  default     = {}
}

variable "service_2_name" {
  type        = string
  description = "Service 2 name"
}

variable "service_2_list" {
  type        = list(string)
  description = "Service 2 list"
  default     = []
}

variable "service_2_map" {
  type        = map(string)
  description = "Service 2 map"
  default     = {}
}
