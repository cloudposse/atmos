# Local-only component with a deliberate delay, used to manually verify whether Ctrl-C during a
# streaming `apply --ui` actually terminates the underlying terraform subprocess.

terraform {
  required_version = ">= 1.0.0"
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
    time = {
      source  = "hashicorp/time"
      version = ">= 0.9.0"
    }
  }
}

variable "sleep_duration" {
  type    = string
  default = "20s"
}

resource "null_resource" "before" {}

resource "time_sleep" "wait" {
  depends_on      = [null_resource.before]
  create_duration = var.sleep_duration
}

resource "null_resource" "after" {
  depends_on = [time_sleep.wait]
}
