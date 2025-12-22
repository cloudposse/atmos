variable "use_existing_security_groups" {
  type        = bool
  description = <<-EOT
    DEPRECATED: Use `create_security_group` instead.
    Historical description: Flag to enable/disable creation of Security Group in the module.
    Set to `true` to disable Security Group creation and provide a list of existing security Group IDs in `existing_security_groups` to place the cluster into.
    Historical default: `false`
    EOT
  default     = null
}

variable "existing_security_groups" {
  type        = list(string)
  default     = []
  description = <<-EOT
    DEPRECATED: Use `associated_security_group_ids` instead.
    Historical description: List of existing Security Group IDs to place the cluster into.
    Set `use_existing_security_groups` to `true` to enable using `existing_security_groups` as Security Groups for the cluster.
    EOT
}

variable "allowed_security_groups" {
  type        = list(string)
  default     = []
  description = <<-EOT
    DEPRECATED: Use `allowed_security_group_ids` instead.
    EOT
}

variable "allowed_cidr_blocks" {
  type        = list(string)
  default     = []
  description = <<-EOT
    DEPRECATED: Use `additional_security_group_rules` instead.
    Historical description: List of CIDR blocks that are allowed ingress to the cluster's Security Group created in the module
    EOT
}

variable "egress_cidr_blocks" {
  type        = list(any)
  default     = null
  description = <<-EOT
    DEPRECATED: Use `allow_all_egress` and `additional_security_group_rules` instead.
    Historical description: Outbound traffic address.
    Historical default: ["0.0.0.0/0"]
    EOT
}

locals {
  # Use the legacy egress rule unless:
  # - var.egress_cidr_blocks is null, which means use the default, which is allow all egress
  # - var.allow_all_egress is true, which explicitly means allow all egress
  # - var.egress_cidr_blocks is exactly ["0.0.0.0/0"], which we interpret to mean "allow all egress"
  use_legacy_egress = !(var.egress_cidr_blocks == null || var.allow_all_egress == true || (
    try(length(var.egress_cidr_blocks), 0) == 1 && try(var.egress_cidr_blocks[0], "") == "0.0.0.0/0")
  )

  # If var.allow_all_egress is null, default to true unless some alternate legacy rule was provided
  allow_all_egress = var.allow_all_egress == null ? !local.use_legacy_egress : var.allow_all_egress
}
