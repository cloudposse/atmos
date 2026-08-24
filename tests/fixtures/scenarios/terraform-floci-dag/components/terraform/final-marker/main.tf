terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region                      = var.aws_region
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    ssm = var.aws_endpoint_url
  }
}

variable "aws_region" {
  type = string
}

variable "aws_endpoint_url" {
  type = string
}

variable "test_id" {
  type = string
}

variable "marker_key" {
  type = string
}

variable "marker_value" {
  type = string
}

resource "aws_ssm_parameter" "marker" {
  name  = "/atmos/pr5/floci/${var.test_id}/${var.marker_key}"
  type  = "String"
  value = var.marker_value
}

output "marker_name" {
  value = aws_ssm_parameter.marker.name
}
