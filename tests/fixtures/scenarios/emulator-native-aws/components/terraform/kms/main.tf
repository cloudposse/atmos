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

locals {
  id = "emu-native-${var.stage}-${var.name}"
}

resource "aws_kms_key" "this" {
  description             = "Encryption key for ${local.id}"
  deletion_window_in_days = 7
}

resource "aws_kms_alias" "this" {
  name          = "alias/${local.id}"
  target_key_id = aws_kms_key.this.key_id
}

output "key_arn" {
  value       = aws_kms_key.this.arn
  description = "ARN of the KMS key."
}
