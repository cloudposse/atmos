variable "stage" {
  description = "Stage. Used to define an Atmos stack."
  type        = string
  default     = "test"
}

variable "exit_code" {
  description = "Exit code used to simulate the command's exit status."
  type        = number
  default     = 0
}

// Use the built-in provider so exit-code tests do not depend on registry downloads.
resource "terraform_data" "fail_on_second_apply" {
  triggers_replace = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = "exit ${var.exit_code}"
  }
}
