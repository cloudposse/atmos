terraform {
  required_providers {
    local = {
      source = "hashicorp/local"
    }
  }
}

variable "db_name" {
  type = string
}

variable "report" {
  type = string
}

variable "child_name" {
  type = string
}

# Stand-in for an RDS instance: `output` mirrors a provider-maintained attribute.
resource "terraform_data" "db" {
  input = var.db_name
}

# Exercises the content hash attributes hidden by earlier editions.
resource "local_file" "report" {
  filename = "${path.module}/report.txt"
  content  = var.report
}

module "child" {
  source = "./modules/child"
  name   = var.child_name
}
