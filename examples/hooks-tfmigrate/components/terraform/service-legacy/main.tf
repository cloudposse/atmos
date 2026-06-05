resource "random_pet" "legacy" {
  length = 2

  keepers = {
    environment = var.environment
  }
}

output "pet_name" {
  value = random_pet.legacy.id
}
