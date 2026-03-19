resource "random_id" "foo" {
  count = var.enabled ? 1 : 0
  keepers = {
    # Generate a new id each time we switch to a new seed
    seed = "${module.this.id}-${var.example}"
  }
  byte_length = 8

  lifecycle {
    ignore_changes = [
      keepers["timestamp"]
    ]
  }
}

locals {
  failure = var.enabled && var.enable_failure ? file("Failed because failure mode is enabled") : null
}

data "validation_warning" "warn" {
  count     = var.enable_warning ? 1 : 0
  condition = true
  summary   = "Test warning summary"
  details   = "Test warning details"
}

provider "validation" {
  # Configuration options
}
