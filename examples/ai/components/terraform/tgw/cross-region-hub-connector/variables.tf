# Transit Gateway Cross-Region Hub Connector Variables

variable "peer_region" {
  type        = string
  description = "AWS region of the peer Transit Gateway"
  default     = "us-east-1"
}

variable "auto_accept_peering" {
  type        = bool
  description = "Auto-accept peering attachment requests"
  default     = true
}

variable "tags" {
  type        = map(string)
  description = "Tags to apply to all resources"
  default     = {}
}
