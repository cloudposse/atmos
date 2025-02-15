terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
  }
}

variable "stage" {
  description = "The file used as a flag to indicate that Terraform has already been applied."
  type        = string
  default     = "/tmp/terraform_once"
}

resource "null_resource" "run_once" {
  # Use a changing trigger to force the provisioner to run every time.
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<-EOT
      if [ -f "${var.stage}" ]; then
        echo "Flag file exists. Exiting with error as intended."
        exit 1
      else
        echo "Flag file not found. Creating flag file."
        touch "${var.stage}"
      fi
    EOT
  }
}
