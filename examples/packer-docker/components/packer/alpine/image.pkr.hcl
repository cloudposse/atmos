# Packer template that builds a local container image using Packer's Docker
# builder.
#
# This builds and commits an image entirely against the local Docker (or
# Podman) daemon. No cloud credentials are involved and nothing is pushed or
# tagged to a registry — there is intentionally no post-processor.
#
# `atmos packer init` downloads the `docker` plugin declared below (network
# access only, no daemon required). `atmos packer build` pulls alpine:3.19,
# runs the shell provisioner, and commits the result as a new local image.
#
# References:
#   - https://developer.hashicorp.com/packer/integrations/hashicorp/docker
#   - https://developer.hashicorp.com/packer/integrations/hashicorp/docker/latest/components/builder/docker

packer {
  required_plugins {
    docker = {
      source  = "github.com/hashicorp/docker"
      version = "~> 1"
    }
  }
}

# ---------------------------------------------------------------------------
# Input variables (supplied by the Atmos stack — see stacks/alpine.yaml).
# ---------------------------------------------------------------------------

variable "message" {
  type        = string
  description = "Message the shell provisioner writes into the image."
  default     = "Hello from Atmos + Packer + Docker!"
}

# `stage` is Atmos's context var (see stacks/alpine.yaml — it drives the
# stack's name via `name_template`). Atmos passes every stack var through to
# Packer's var-file, so it must be declared here even though this template
# doesn't otherwise use it.
variable "stage" {
  type        = string
  description = "Atmos stack context var (unused by this template)."
  default     = null
}

# ---------------------------------------------------------------------------
# Builder — pull alpine:3.19 and commit the provisioned container as a new
# local image (commit = true). No image name/tag is set, so nothing is
# published anywhere; the image only exists in the local Docker/Podman
# image store.
# ---------------------------------------------------------------------------

source "docker" "example" {
  image  = "alpine:3.19"
  commit = true
}

# ---------------------------------------------------------------------------
# Build — run one shell provisioner that writes and prints a marker file so
# the build's output is visibly demonstrable.
# ---------------------------------------------------------------------------

build {
  sources = ["source.docker.example"]

  provisioner "shell" {
    inline = [
      "echo '${var.message}' > /etc/atmos-demo.txt",
      "cat /etc/atmos-demo.txt",
    ]
  }
}
