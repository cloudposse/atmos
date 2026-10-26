# Transit Gateway Attachment Variables

variable "appliance_mode_support" {
  type        = bool
  description = "Enable appliance mode for the attachment"
  default     = false
}

variable "dns_support" {
  type        = bool
  description = "Enable DNS support for the attachment"
  default     = true
}

variable "transit_gateway_default_route_table_association" {
  type        = bool
  description = "Associate with default route table"
  default     = true
}

variable "transit_gateway_default_route_table_propagation" {
  type        = bool
  description = "Propagate routes to default route table"
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to all resources"
  default     = {}
}
