# Base AWS provider configuration.
#
# Atmos augments this with a generated `providers_override.tf.json` (credentials,
# path-style addressing, and skip flags) so the same component runs against the local
# AWS emulator (Floci) or real AWS without edits. `region` is declared as a variable and
# referenced here so the value supplied by the region mixin is consumed (no "undeclared
# variable" warning) without being flagged as unused.
provider "aws" {
  region = var.region
}
