variable "project_id_1" {
  type        = string
  description = "GCP Project ID 1"
}

variable "project_id_2" {
  type        = string
  description = "GCP Project ID 2"
}

variable "region_1" {
  type        = string
  description = "GCP region 1"
}

variable "region_2" {
  type        = string
  description = "GCP region 2"
}

variable "auto_create_subnetworks" {
  type        = bool
  description = "A boolean flag to auto create subnetworks"
  default     = true
}

variable "subnets" {
  type = list(object({
    name                = string
    ip_cidr_range       = string
    region              = optional(string)
    description         = optional(string)
    secondary_ip_ranges = optional(list(object({ range_name = string, ip_cidr_range = string })))

  }))
  default = []
}
