resource "random_pet" "service" {
  length = 2

  keepers = {
    environment = var.environment
  }
}

output "pet_name" {
  value = random_pet.service.id
}
