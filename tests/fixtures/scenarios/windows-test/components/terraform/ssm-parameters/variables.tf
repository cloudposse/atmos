variable "region" {
  type        = string
  description = "AWS Region"
}

variable "sops_source_file" {
  type        = string
  description = "The relative path to the SOPS file which is consumed as the source for creating parameter resources."
  default     = ""
}

variable "sops_source_key" {
  type        = string
  description = "The SOPS key to pull from the source file."
  default     = ""
}

variable "kms_arn" {
  type        = string
  description = "The ARN of a KMS key used to encrypt and decrypt SecretString values"
  default     = ""
}

variable "params" {
  type = map(object({
    value                = string
    description          = string
    overwrite            = optional(bool, false)
    tier                 = optional(string, "Standard")
    type                 = string
    ignore_value_changes = optional(bool, false)
  }))
  description = "A map of parameter values to write to SSM Parameter Store"
}
