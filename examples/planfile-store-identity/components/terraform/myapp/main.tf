terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
  }
}

variable "stage" {
  type        = string
  description = "Stage/environment name"
}

resource "null_resource" "example" {
  triggers = {
    stage = var.stage
  }
}

output "stage" {
  description = "The stage this component was deployed to"
  value       = var.stage
}
