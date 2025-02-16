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

resource "null_resource" "run_once" {
  # Force the provisioner to run on every apply.
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<EOF
python -c "import os, sys, tempfile; flag = './terraform_once.tfstate.temp'; print('Using flag file:', flag); sys.exit(1) if os.path.exists(flag) else (open(flag,'w').close() or sys.exit(0))"
EOF
  }
}
