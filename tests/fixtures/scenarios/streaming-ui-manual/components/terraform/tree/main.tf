# Local-only component (null_resource/local_file, no cloud provider) used to manually exercise
# the terraform streaming UI's resource tree, dependency rendering, and attribute-diff display
# across create/update/destroy actions.

terraform {
  required_version = ">= 1.0.0"
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
    local = {
      source  = "hashicorp/local"
      version = ">= 2.0.0"
    }
  }
}

variable "enabled" {
  type    = bool
  default = true
}

variable "instance_count" {
  type        = number
  default     = 2
  description = "Change this between applies to produce create/update/destroy diffs in the tree."
}

resource "null_resource" "root" {
  triggers = {
    count = var.instance_count
  }
}

resource "null_resource" "leaf" {
  count = var.instance_count

  triggers = {
    index = count.index
  }

  depends_on = [null_resource.root]
}

resource "local_file" "manifest" {
  filename = "${path.module}/.manifest-${var.instance_count}.txt"
  content  = "instance_count=${var.instance_count}"

  depends_on = [null_resource.leaf]
}

output "instance_count" {
  value = var.instance_count
}
