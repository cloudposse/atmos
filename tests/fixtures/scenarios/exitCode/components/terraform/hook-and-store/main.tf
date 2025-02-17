terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
  }
}

variable "stage" {
  description = "Stage. Used to define an Atmos stack."
  type        = string
  default     = "test"
}

variable "exit_code" {
  description = "Stage. Used to define an Atmos stack."
  type        =  number
  default     =  0
}


resource "null_resource" "fail_on_second_apply" {
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = "exit ${var.exit_code}"
  }
}
