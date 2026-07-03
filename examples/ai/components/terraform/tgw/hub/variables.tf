# Transit Gateway Hub Variables

variable "amazon_side_asn" {
  type        = number
  description = "Private ASN for the Transit Gateway"
  default     = 64512
}

variable "auto_accept_shared_attachments" {
  type        = bool
  description = "Auto-accept shared attachments from other accounts"
  default     = true
}

variable "default_route_table_association" {
  type        = bool
  description = "Automatically associate attachments with the default route table"
  default     = true
}

variable "default_route_table_propagation" {
  type        = bool
  description = "Automatically propagate routes to the default route table"
  default     = true
}

variable "dns_support" {
  type        = bool
  description = "Enable DNS support on the Transit Gateway"
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to all resources"
  default     = {}
}
