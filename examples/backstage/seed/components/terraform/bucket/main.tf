terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.52.0"
    }
  }
}

variable "stage" {
  type = string
}
variable "service_name" {
  type = string
}

resource "aws_s3_bucket" "service" {
  bucket        = "atmos-backstage-${var.stage}-${var.service_name}"
  force_destroy = true
  tags = {
    Service   = var.service_name
    ManagedBy = "Atmos"
  }
}

output "bucket_name" {
  value = aws_s3_bucket.service.id
}
