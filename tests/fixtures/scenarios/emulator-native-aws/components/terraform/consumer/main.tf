terraform {
  required_version = ">= 1.3.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

variable "name" {
  type        = string
  description = "Component name used to build the resource identifier."
}

variable "stage" {
  type        = string
  description = "Stage name."
  default     = ""
}

variable "kms_key_arn" {
  type        = string
  description = "KMS key ARN used to encrypt the SecureString parameter."
  default     = ""
}

variable "upstream_bucket" {
  type        = string
  description = "Producer bucket id, resolved cross-component via the store (!store)."
}

variable "app_secret" {
  type        = string
  description = "Application secret, resolved via the secrets engine (!secret)."
  sensitive   = true
}

locals {
  id     = "emu-native-${var.stage}-${var.name}"
  prefix = "/emu-native/${var.stage}/${var.name}"
}

# Writes the store-resolved upstream coordinate back as a plain parameter — proving
# the !store value flowed through.
resource "aws_ssm_parameter" "upstream_bucket" {
  name  = "${local.prefix}/upstream_bucket"
  type  = "String"
  value = var.upstream_bucket
}

# Writes the secret to a SecureString — proving the !secret value arrived off-disk
# (as TF_VAR_app_secret) and the in-process secret store reached the emulator.
resource "aws_ssm_parameter" "app_secret" {
  name   = "${local.prefix}/app_secret"
  type   = "SecureString"
  value  = var.app_secret
  key_id = var.kms_key_arn != "" ? var.kms_key_arn : null
}

output "received_bucket" {
  value       = var.upstream_bucket
  description = "The producer bucket id received via !store (asserted by the E2E test)."
}
