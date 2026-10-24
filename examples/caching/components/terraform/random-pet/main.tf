# A registry-sourced module so module-registry traffic flows through the cache:
# its version listing and download resolution route through the proxy's module
# mirror and are cached. It declares no providers and makes no external calls —
# it just normalizes a label string from the inputs.
module "label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  stage = var.stage
  name  = "pet"
}

resource "random_pet" "this" {
  length    = var.length
  separator = var.separator

  # Re-generate the pet name whenever the resolved label changes.
  keepers = {
    label = module.label.id
  }
}

# tls: generate a throwaway key (no external calls, no credentials).
resource "tls_private_key" "this" {
  algorithm = "ED25519"
}

# null: a resource that re-runs when the pet name changes.
resource "null_resource" "this" {
  triggers = {
    pet = random_pet.this.id
  }
}

# local: write the generated name to a per-stage file on apply.
resource "local_file" "this" {
  content  = random_pet.this.id
  filename = "${path.module}/pet.${module.label.id}.txt"
}
