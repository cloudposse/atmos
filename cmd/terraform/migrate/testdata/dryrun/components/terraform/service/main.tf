resource "random_pet" "service" {
  length = 2

  keepers = {
    environment = var.environment
  }
}
